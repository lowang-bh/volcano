/*
Copyright 2023 The Kubernetes Authors.

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
	"reflect"
	"testing"

	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"

	schedulingv1 "volcano.sh/apis/pkg/apis/scheduling/v1beta1"
	"volcano.sh/volcano/cmd/scheduler/app/options"
	"volcano.sh/volcano/pkg/kube"
	"volcano.sh/volcano/pkg/scheduler/cache"
	"volcano.sh/volcano/pkg/scheduler/conf"
	"volcano.sh/volcano/pkg/scheduler/framework"
	"volcano.sh/volcano/pkg/scheduler/plugins/predicates"
	"volcano.sh/volcano/pkg/scheduler/plugins/proportion"
	"volcano.sh/volcano/pkg/scheduler/util"
)

func TestBackfill(t *testing.T) {
	framework.RegisterPluginBuilder("predicates", predicates.New)
	framework.RegisterPluginBuilder("nodeorder", proportion.New)

	defer framework.CleanupPluginBuilders()
	options.ServerOpts = options.NewServerOption()
	options.ServerOpts.AddFlags(pflag.CommandLine)
	options.ServerOpts.RegisterOptions()

	config, err := kube.BuildConfig(options.ServerOpts.KubeClientOptions)
	if err != nil {
		return
	}

	sc := cache.New(config, options.ServerOpts.SchedulerNames, options.ServerOpts.DefaultQueue, options.ServerOpts.NodeSelector)
	schedulerCache := sc.(*cache.SchedulerCache)
	tests := []struct {
		name      string
		podGroups []*schedulingv1.PodGroup
		pods      []*v1.Pod
		nodes     []*v1.Node
		queues    []*schedulingv1.Queue
		expected  map[string]string
	}{
		{
			name: "pod with node selector",
			podGroups: []*schedulingv1.PodGroup{
				util.BuildPodGroup("pg1", "c1", "c1", 0, nil, schedulingv1.PodGroupInqueue),
			},
			pods: []*v1.Pod{
				util.BuildPod("c1", "p1", "", v1.PodPending, nil, "pg1", make(map[string]string), map[string]string{"platform": "cpu"}),
				util.BuildPod("c1", "p2", "", v1.PodPending, nil, "pg1", make(map[string]string), map[string]string{"platform": "gpu"}),
			},
			nodes: []*v1.Node{
				util.BuildNode("n1", util.BuildResourceList("2", "4Gi"), map[string]string{"platform": "cpu"}),
				util.BuildNode("n2", util.BuildResourceList("2", "4Gi"), map[string]string{"platform": "gpu"}),
			},
			queues: []*schedulingv1.Queue{
				util.BuildQueue("c1", 1, nil),
			},
			expected: map[string]string{
				"c1/p1": "n1",
				"c1/p2": "n2",
			},
		},
		{
			name: "pods has anti-affinity with existing pods",
			podGroups: []*schedulingv1.PodGroup{
				util.BuildPodGroup("pg1", "c1", "q1", 0, nil, schedulingv1.PodGroupInqueue),
				util.BuildPodGroup("pg2", "c2", "q2", 0, nil, schedulingv1.PodGroupInqueue),
			},
			pods: []*v1.Pod{
				util.BuildPod("c1", "pg1-pod-1", "n1", v1.PodRunning, nil, "pg1", map[string]string{"role": "ps"}, make(map[string]string)),
				util.BuildPod("c2", "pg2-pod-1", "", v1.PodPending, nil, "pg2", make(map[string]string), make(map[string]string)),
				util.BuildPod("c2", "pg2-pod-2", "", v1.PodPending, nil, "pg2", make(map[string]string), make(map[string]string)),
			},
			nodes: []*v1.Node{
				util.BuildNode("n1", util.BuildResourceList("2", "4G"), make(map[string]string)),
			},
			queues: []*schedulingv1.Queue{
				util.BuildQueue("q1", 1, nil),
				util.BuildQueue("q2", 1, nil),
			},
			expected: map[string]string{
				"c2/pg2-p-1": "n1",
				"c1/pg1-p-1": "n1",
			},
		},
		{
			name: "high priority queue should not block others",
			podGroups: []*schedulingv1.PodGroup{
				util.BuildPodGroup("pg1", "c1", "c1", 0, nil, schedulingv1.PodGroupInqueue),
				util.BuildPodGroup("pg2", "c1", "c2", 0, nil, schedulingv1.PodGroupInqueue),
			},

			pods: []*v1.Pod{
				util.BuildPod("c1", "p1", "", v1.PodPending, nil, "pg1", make(map[string]string), make(map[string]string)),
				util.BuildPod("c1", "p2", "", v1.PodPending, nil, "pg2", make(map[string]string), make(map[string]string)),
			},
			nodes: []*v1.Node{
				util.BuildNode("n1", util.BuildResourceList("2", "4G"), make(map[string]string)),
			},
			queues: []*schedulingv1.Queue{
				util.BuildQueue("c1", 1, nil),
				util.BuildQueue("c2", 1, nil),
			},
			expected: map[string]string{
				"c1/p2": "n1",
			},
		},
	}

	backfill := New()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			binder := &util.FakeBinder{
				Binds:   map[string]string{},
				Channel: make(chan string),
			}

			for _, node := range test.nodes {
				schedulerCache.AddNode(node)
			}
			for _, pod := range test.pods {
				schedulerCache.AddPod(pod)
			}

			for _, ss := range test.podGroups {
				schedulerCache.AddPodGroupV1beta1(ss)
			}

			for _, q := range test.queues {
				schedulerCache.AddQueueV1beta1(q)
			}

			trueValue := true
			ssn := framework.OpenSession(schedulerCache, []conf.Tier{
				{
					Plugins: []conf.PluginOption{
						{
							Name:             "predicates",
							EnabledPredicate: &trueValue,
						},
						{
							Name:             "nodeorder",
							EnabledNodeOrder: &trueValue,
							EnabledBestNode:  &trueValue,
							Arguments: map[string]interface{}{
								"nodeaffinity.weight":    2,
								"podaffinity.weight":     1,
								"lleastrequested.weight": 1,
							},
						},
					},
				},
			}, nil)
			defer framework.CloseSession(ssn)

			backfill.Execute(ssn)

			if !reflect.DeepEqual(test.expected, binder.Binds) {
				t.Errorf("expected: %v, got %v ", test.expected, binder.Binds)
			}
		})
	}
}
