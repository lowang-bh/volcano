/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package backfill

import (
	"time"

	"k8s.io/klog/v2"

	"volcano.sh/volcano/pkg/scheduler/api"
	"volcano.sh/volcano/pkg/scheduler/framework"
	"volcano.sh/volcano/pkg/scheduler/metrics"
	"volcano.sh/volcano/pkg/scheduler/util"
)

type Action struct{}

func New() *Action {
	return &Action{}
}

func (backfill *Action) Name() string {
	return "backfill"
}

func (backfill *Action) Initialize() {}

func (backfill *Action) Execute(ssn *framework.Session) {
	klog.V(5).Infof("Enter Backfill ...")
	defer klog.V(5).Infof("Leaving Backfill ...")

	// TODO (k82cn): When backfill, it's also need to balance between Queues.
	for _, job := range ssn.Jobs {
		if job.IsPending() {
			continue
		}

		if vr := ssn.JobValid(job); vr != nil && !vr.Pass {
			klog.V(4).Infof("Job <%s/%s> Queue <%s> skip backfill, reason: %v, message %v", job.Namespace, job.Name, job.Queue, vr.Reason, vr.Message)
			continue
		}

		ph := util.NewPredicateHelper()

		for _, task := range job.TaskStatusIndex[api.Pending] {
			if task.InitResreq.IsEmpty() {
				allocated := false
				fe := api.NewFitErrors()

				if err := ssn.PrePredicateFn(task); err != nil {
					klog.V(3).Infof("PrePredicate for task %s/%s failed for: %v", task.Namespace, task.Name, err)
					for _, ni := range ssn.Nodes {
						fe.SetNodeError(ni.Name, err)
					}
					job.NodesFitErrors[task.UID] = fe
					break
				}

				predicateNodes, fitErrors := ph.PredicateNodes(task, ssn.NodeList, ssn.PredicateFn, true)
				if len(predicateNodes) == 0 {
					klog.V(3).Infof("Predicates failed for task <%s/%s>", task.Namespace, task.Name)
					job.NodesFitErrors[task.UID] = fitErrors
					break
				}

				node := predicateNodes[0]
				if len(predicateNodes) > 1 {
					nodeScores := util.PrioritizeNodes(task, predicateNodes, ssn.BatchNodeOrderFn, ssn.NodeOrderMapFn, ssn.NodeOrderReduceFn)
					node = ssn.BestNodeFn(task, nodeScores)
					if node == nil {
						node = util.SelectBestNode(nodeScores)
					}
				}
				// As task did not request resources, so it only need to meet predicates.
				// Done (k82cn): need to prioritize nodes to avoid pod hole.
				klog.V(3).Infof("Binding Task <%v/%v> to node <%v>", task.Namespace, task.Name, node.Name)
				if err := ssn.Allocate(task, node); err != nil {
					klog.Errorf("Failed to bind Task %v on %v in Session %v", task.UID, node.Name, ssn.UID)
					fe.SetNodeError(node.Name, err)
					continue
				}

				metrics.UpdateE2eSchedulingDurationByJob(job.Name, string(job.Queue), job.Namespace, metrics.Duration(job.CreationTimestamp.Time))
				metrics.UpdateE2eSchedulingLastTimeByJob(job.Name, string(job.Queue), job.Namespace, time.Now())
				allocated = true

				if !allocated {
					job.NodesFitErrors[task.UID] = fe
				}
			}
			// TODO (k82cn): backfill for other case.
		}
	}
}

func (backfill *Action) UnInitialize() {}
