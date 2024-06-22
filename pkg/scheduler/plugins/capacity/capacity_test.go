/*
Copyright 2024 The Volcano Authors.

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

package capacity

import (
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	schedulingv1beta1 "volcano.sh/apis/pkg/apis/scheduling/v1beta1"

	"volcano.sh/volcano/cmd/scheduler/app/options"
	"volcano.sh/volcano/pkg/scheduler/actions/allocate"
	"volcano.sh/volcano/pkg/scheduler/actions/enqueue"
	"volcano.sh/volcano/pkg/scheduler/actions/reclaim"
	"volcano.sh/volcano/pkg/scheduler/api"
	"volcano.sh/volcano/pkg/scheduler/conf"
	"volcano.sh/volcano/pkg/scheduler/framework"
	"volcano.sh/volcano/pkg/scheduler/plugins/predicates"
	"volcano.sh/volcano/pkg/scheduler/uthelper"
	"volcano.sh/volcano/pkg/scheduler/util"
)

func TestMain(m *testing.M) {
	options.Default()
	os.Exit(m.Run())
}

func Test_capacityPlugin_OnSessionOpen(t *testing.T) {
	plugins := map[string]framework.PluginBuilder{PluginName: New, predicates.PluginName: predicates.New}
	trueValue := true
	actions := []framework.Action{allocate.New(), reclaim.New()}

	// nodes
	n1 := util.BuildNode("n1", api.BuildResourceList("2", "4Gi", []api.ScalarResource{{Name: "pods", Value: "10"}}...), map[string]string{"selector": "worker"})
	n2 := util.BuildNode("n2", api.BuildResourceList("2", "4Gi", []api.ScalarResource{{Name: "pods", Value: "10"}}...), map[string]string{})

	// resources for test case 0
	// pod
	p1 := util.BuildPod("ns1", "p1", "n1", corev1.PodRunning, api.BuildResourceList("1", "1Gi"), "pg1", make(map[string]string), make(map[string]string))
	p2 := util.BuildPod("ns1", "p2", "", corev1.PodPending, api.BuildResourceList("1", "1Gi"), "pg2", make(map[string]string), map[string]string{"selector": "worker"})
	// podgroup
	pg1 := util.BuildPodGroup("pg1", "ns1", "q1", 1, nil, schedulingv1beta1.PodGroupRunning)
	pg2 := util.BuildPodGroup("pg2", "ns1", "q1", 1, nil, schedulingv1beta1.PodGroupInqueue)
	// queue
	queue1 := util.BuildQueueWithResourcesQuantity("q1", nil, api.BuildResourceList("2", "2Gi"))

	// resources for test case 1
	// pod
	p3 := util.BuildPod("ns1", "p3", "n1", corev1.PodRunning, api.BuildResourceList("1", "1Gi"), "pg3", make(map[string]string), make(map[string]string))
	p4 := util.BuildPod("ns1", "p4", "", corev1.PodPending, api.BuildResourceList("1", "1Gi"), "pg4", make(map[string]string), make(map[string]string))
	// podgroup
	pg3 := util.BuildPodGroup("pg3", "ns1", "q2", 1, nil, schedulingv1beta1.PodGroupRunning)
	pg4 := util.BuildPodGroup("pg4", "ns1", "q2", 1, nil, schedulingv1beta1.PodGroupInqueue)
	// queue
	queue2 := util.BuildQueueWithResourcesQuantity("q2", nil, api.BuildResourceList("1.5", "1.5Gi"))

	// resources for test case 2
	// pod
	p5 := util.BuildPod("ns1", "p5", "n1", corev1.PodRunning, api.BuildResourceList("2", "4Gi"), "pg5", map[string]string{schedulingv1beta1.PodPreemptable: "false"}, make(map[string]string))
	p6 := util.BuildPod("ns1", "p6", "n2", corev1.PodRunning, api.BuildResourceList("2", "4Gi"), "pg5", make(map[string]string), make(map[string]string))
	p7 := util.BuildPod("ns1", "p7", "", corev1.PodPending, api.BuildResourceList("2", "4Gi"), "pg6", make(map[string]string), make(map[string]string))
	// podgroup
	pg5 := util.BuildPodGroup("pg5", "ns1", "q3", 1, nil, schedulingv1beta1.PodGroupRunning)
	pg6 := util.BuildPodGroup("pg6", "ns1", "q4", 1, nil, schedulingv1beta1.PodGroupInqueue)
	// queue
	queue3 := util.BuildQueueWithResourcesQuantity("q3", api.BuildResourceList("2", "4Gi"), nil)
	queue4 := util.BuildQueueWithResourcesQuantity("q4", api.BuildResourceList("2", "4Gi"), nil)

	tests := []uthelper.TestCommonStruct{
		{
			Name:      "case0: Pod allocatable when queue has not exceed capability",
			Plugins:   plugins,
			Pods:      []*corev1.Pod{p1, p2},
			Nodes:     []*corev1.Node{n1, n2},
			PodGroups: []*schedulingv1beta1.PodGroup{pg1, pg2},
			Queues:    []*schedulingv1beta1.Queue{queue1},
			ExpectBindMap: map[string]string{
				"ns1/p2": "n1",
			},
			ExpectBindsNum: 1,
		},
		{
			Name:           "case1: Pod not allocatable when queue exceed queue capability",
			Plugins:        plugins,
			Pods:           []*corev1.Pod{p3, p4},
			Nodes:          []*corev1.Node{n1, n2},
			PodGroups:      []*schedulingv1beta1.PodGroup{pg3, pg4},
			Queues:         []*schedulingv1beta1.Queue{queue2},
			ExpectBindsNum: 0,
		},
		{
			Name:      "case2: Can reclaim from other queues when allocated < deserved",
			Plugins:   plugins,
			Pods:      []*corev1.Pod{p5, p6, p7},
			Nodes:     []*corev1.Node{n1, n2},
			PodGroups: []*schedulingv1beta1.PodGroup{pg5, pg6},
			Queues:    []*schedulingv1beta1.Queue{queue3, queue4},
			ExpectPipeLined: map[string][]string{
				"ns1/pg6": {"n2"},
			},
			ExpectEvicted:  []string{"ns1/p6"},
			ExpectEvictNum: 1,
		},
	}

	tiers := []conf.Tier{
		{
			Plugins: []conf.PluginOption{
				{
					Name:               PluginName,
					EnabledAllocatable: &trueValue,
					EnablePreemptive:   &trueValue,
					EnabledReclaimable: &trueValue,
				},
				{
					Name:             predicates.PluginName,
					EnabledPredicate: &trueValue,
				},
			},
		},
	}
	for i, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			test.RegisterSession(tiers, nil)
			defer test.Close()
			test.Run(actions)
			if err := test.CheckAll(i); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEnqueueAndAllocable(t *testing.T) {
	// nodes
	n1 := util.BuildNode("n1", api.BuildResourceList("3", "3G", []api.ScalarResource{{Name: "pods", Value: "10"}}...), nil)
	n2 := util.BuildNode("n2", api.BuildResourceList("3", "3G", []api.ScalarResource{{Name: "pods", Value: "10"}}...), nil)

	// resources
	res1c3g := api.BuildResourceList("1", "3G")
	res3c1g := api.BuildResourceList("3", "1G")
	res1c0g := api.BuildResourceList("1", "0G")
	res0c1g := api.BuildResourceList("0", "1G")
	res1c1g := api.BuildResourceList("1", "1G")
	// pod
	p1 := util.BuildPod("ns1", "pod1", "n1", corev1.PodRunning, res1c3g, "pg1", nil, nil)
	p2 := util.BuildPod("ns1", "pod2", "n2", corev1.PodRunning, res3c1g, "pg2", nil, nil)
	p3 := util.BuildPod("ns1", "pod3", "", corev1.PodPending, res1c0g, "pg3", nil, nil)
	p4 := util.BuildPod("ns1", "pod4", "", corev1.PodPending, res0c1g, "pg4", nil, nil)
	p5 := util.BuildPod("ns1", "pod5", "", corev1.PodPending, res1c1g, "pg5", nil, nil)

	// podgroup
	pg1 := util.BuildPodGroup("pg1", "ns1", "q1", 1, nil, schedulingv1beta1.PodGroupRunning)
	pg2 := util.BuildPodGroup("pg2", "ns1", "q2", 1, nil, schedulingv1beta1.PodGroupRunning)
	pg3 := util.BuildPodGroup("pg3", "ns1", "q1", 1, nil, schedulingv1beta1.PodGroupPending)
	pg4 := util.BuildPodGroup("pg4", "ns1", "q2", 1, nil, schedulingv1beta1.PodGroupPending)
	pg5 := util.BuildPodGroup("pg5", "ns1", "q1", 1, nil, schedulingv1beta1.PodGroupPending)
	pg1.Spec.MinResources = &res1c3g
	pg2.Spec.MinResources = &res3c1g
	pg3.Spec.MinResources = &res1c0g
	pg4.Spec.MinResources = &res0c1g
	pg5.Spec.MinResources = &res1c1g

	queue1 := util.BuildQueueWithResourcesQuantity("q1", api.BuildResourceList("2", "2G"), api.BuildResourceList("2", "2G"))
	queue2 := util.BuildQueueWithResourcesQuantity("q2", api.BuildResourceList("2", "2G"), api.BuildResourceList("3", "3G"))

	plugins := map[string]framework.PluginBuilder{PluginName: New}
	trueValue := true
	tiers := []conf.Tier{
		{
			Plugins: []conf.PluginOption{
				{
					Name:               PluginName,
					EnabledAllocatable: &trueValue,
					EnablePreemptive:   &trueValue,
					EnabledOverused:    &trueValue,
					EnabledJobEnqueued: &trueValue,
				},
			},
		},
	}
	tests := []uthelper.TestCommonStruct{
		{
			Name:           "case0: memory exceed derserved, job only request cpu can be enqueued and allocated",
			Plugins:        plugins,
			Pods:           []*corev1.Pod{p1, p2, p3},
			Nodes:          []*corev1.Node{n1, n2},
			PodGroups:      []*schedulingv1beta1.PodGroup{pg1, pg2, pg3},
			Queues:         []*schedulingv1beta1.Queue{queue1, queue2},
			ExpectBindsNum: 1,
			ExpectBindMap:  map[string]string{"ns1/pod3": "n1"},
		},
		{
			Name:           "case1: cpu exceed derserved, job only request memory can be enqueued and allocated",
			Plugins:        plugins,
			Pods:           []*corev1.Pod{p1, p2, p4},
			Nodes:          []*corev1.Node{n1, n2},
			PodGroups:      []*schedulingv1beta1.PodGroup{pg1, pg2, pg4},
			Queues:         []*schedulingv1beta1.Queue{queue1, queue2},
			ExpectBindsNum: 1,
			ExpectBindMap:  map[string]string{"ns1/pod4": "n2"},
		},
		{
			Name:           "case2: exceed capacity, can not enqueue",
			Plugins:        plugins,
			Pods:           []*corev1.Pod{p1, p2, p5},
			Nodes:          []*corev1.Node{n1, n2},
			PodGroups:      []*schedulingv1beta1.PodGroup{pg1, pg2, pg5},
			Queues:         []*schedulingv1beta1.Queue{queue1, queue2},
			ExpectBindsNum: 0,
			ExpectBindMap:  map[string]string{},
		},
	}
	actions := []framework.Action{enqueue.New(), allocate.New()}

	for i, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			test.RegisterSession(tiers, nil)
			defer test.Close()
			test.Run(actions)

			if err := test.CheckAll(i); err != nil {
				t.Fatal(err)
			}
		})
	}
}
