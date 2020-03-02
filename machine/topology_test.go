// Copyright 2014 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package machine

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	info "github.com/google/cadvisor/info/v1"
	"github.com/google/cadvisor/utils/sysfs"
	"github.com/google/cadvisor/utils/sysfs/fakesysfs"
	"github.com/stretchr/testify/assert"
)

func TestTopology(t *testing.T) {
	sysFs := &fakesysfs.FakeSysFs{}
	c := sysfs.CacheInfo{
		Size:  32 * 1024,
		Type:  "unified",
		Level: 1,
		Cpus:  2,
	}
	sysFs.SetCacheInfo(c)

	nodesPaths := []string{
		"/fakeSysfs/devices/system/node/node0",
		"/fakeSysfs/devices/system/node/node1",
	}
	sysFs.SetNodesPaths(nodesPaths, nil)

	cpusPaths := map[string][]string{
		"/fakeSysfs/devices/system/node/node0": {
			"/fakeSysfs/devices/system/node/node0/cpu0",
			"/fakeSysfs/devices/system/node/node0/cpu1",
			"/fakeSysfs/devices/system/node/node0/cpu2",
			"/fakeSysfs/devices/system/node/node0/cpu6",
			"/fakeSysfs/devices/system/node/node0/cpu7",
			"/fakeSysfs/devices/system/node/node0/cpu8",
		},
		"/fakeSysfs/devices/system/node/node1": {
			"/fakeSysfs/devices/system/node/node0/cpu3",
			"/fakeSysfs/devices/system/node/node0/cpu4",
			"/fakeSysfs/devices/system/node/node0/cpu5",
			"/fakeSysfs/devices/system/node/node0/cpu9",
			"/fakeSysfs/devices/system/node/node0/cpu10",
			"/fakeSysfs/devices/system/node/node0/cpu11",
		},
	}
	sysFs.SetCPUsPaths(cpusPaths, nil)

	coreThread := map[string][]byte{
		"/fakeSysfs/devices/system/node/node0/cpu0":  []byte("0"),
		"/fakeSysfs/devices/system/node/node0/cpu1":  []byte("1"),
		"/fakeSysfs/devices/system/node/node0/cpu2":  []byte("2"),
		"/fakeSysfs/devices/system/node/node0/cpu3":  []byte("3"),
		"/fakeSysfs/devices/system/node/node0/cpu4":  []byte("4"),
		"/fakeSysfs/devices/system/node/node0/cpu5":  []byte("5"),
		"/fakeSysfs/devices/system/node/node0/cpu6":  []byte("0"),
		"/fakeSysfs/devices/system/node/node0/cpu7":  []byte("1"),
		"/fakeSysfs/devices/system/node/node0/cpu8":  []byte("2"),
		"/fakeSysfs/devices/system/node/node0/cpu9":  []byte("3"),
		"/fakeSysfs/devices/system/node/node0/cpu10": []byte("4"),
		"/fakeSysfs/devices/system/node/node0/cpu11": []byte("5"),
	}
	sysFs.SetCoreThreads(coreThread, nil)

	memTotal := []byte("Node 0 MemTotal:       32817192 kB")
	sysFs.SetMemory(memTotal, nil)

	hugePages := []os.FileInfo{
		&fakesysfs.FileInfo{EntryName: "hugepages-2048kB"},
		&fakesysfs.FileInfo{EntryName: "hugepages-1048576kB"},
	}
	sysFs.SetHugePages(hugePages, nil)

	hugePageNr := map[string][]byte{
		"/fakeSysfs/devices/system/node/node0/hugepages/hugepages-2048kB/nr_hugepages":    []byte("1"),
		"/fakeSysfs/devices/system/node/node0/hugepages/hugepages-1048576kB/nr_hugepages": []byte("1"),
		"/fakeSysfs/devices/system/node/node1/hugepages/hugepages-2048kB/nr_hugepages":    []byte("1"),
		"/fakeSysfs/devices/system/node/node1/hugepages/hugepages-1048576kB/nr_hugepages": []byte("1"),
	}
	sysFs.SetHugePagesNr(hugePageNr, nil)

	topology, numCores, err := GetTopology(sysFs)
	assert.Nil(t, err)

	if numCores != 12 {
		t.Errorf("Expected 12 cores, found %d", numCores)
	}
	expected_topology := []info.Node{}
	numNodes := 2
	numCoresPerNode := 3
	numThreads := 2
	cache := info.Cache{
		Size:  32 * 1024,
		Type:  "unified",
		Level: 1,
	}
	for i := 0; i < numNodes; i++ {
		node := info.Node{Id: i}
		// Copy over Memory from result. TODO(rjnagal): Use memory from fake.
		node.Memory = topology[i].Memory
		// Copy over HugePagesInfo from result. TODO(ohsewon): Use HugePagesInfo from fake.
		node.HugePages = topology[i].HugePages
		for j := 0; j < numCoresPerNode; j++ {
			core := info.Core{Id: i*numCoresPerNode + j}
			core.Caches = append(core.Caches, cache)
			for k := 0; k < numThreads; k++ {
				core.Threads = append(core.Threads, k*numCoresPerNode*numNodes+core.Id)
			}
			node.Cores = append(node.Cores, core)
		}
		expected_topology = append(expected_topology, node)
	}

	if !reflect.DeepEqual(topology, expected_topology) {
		t.Errorf("Expected topology %+v, got %+v", expected_topology, topology)
	}
}

func TestTopologyEmptySysFs(t *testing.T) {
	_, _, err := GetTopology(&fakesysfs.FakeSysFs{})
	assert.NotNil(t, err)
}

func TestTopologyWithNodesWithoutCPU(t *testing.T) {
	sysFs := &fakesysfs.FakeSysFs{}
	nodesPaths := []string{
		"/fakeSysfs/devices/system/node/node0",
		"/fakeSysfs/devices/system/node/node1",
	}
	sysFs.SetNodesPaths(nodesPaths, nil)

	memTotal := []byte("MemTotal:       32817192 kB")
	sysFs.SetMemory(memTotal, nil)

	hugePages := []os.FileInfo{
		&fakesysfs.FileInfo{EntryName: "hugepages-2048kB"},
		&fakesysfs.FileInfo{EntryName: "hugepages-1048576kB"},
	}
	sysFs.SetHugePages(hugePages, nil)

	hugePageNr := map[string][]byte{
		"/fakeSysfs/devices/system/node/node0/hugepages/hugepages-2048kB/nr_hugepages":    []byte("1"),
		"/fakeSysfs/devices/system/node/node0/hugepages/hugepages-1048576kB/nr_hugepages": []byte("1"),
		"/fakeSysfs/devices/system/node/node1/hugepages/hugepages-2048kB/nr_hugepages":    []byte("1"),
		"/fakeSysfs/devices/system/node/node1/hugepages/hugepages-1048576kB/nr_hugepages": []byte("1"),
	}
	sysFs.SetHugePagesNr(hugePageNr, nil)

	topology, numCores, err := GetTopology(sysFs)

	assert.Nil(t, err)
	assert.Equal(t, 0, numCores)

	topologyJSON, err := json.Marshal(topology)
	assert.Nil(t, err)

	expectedTopology := `[
     {
      "caches": null,
      "cores": null,
      "hugepages": [
       {
        "num_pages": 1,
        "page_size": 2048
       },
       {
        "num_pages": 1,
        "page_size": 1048576
       }
      ],
      "memory": 33604804608,
      "node_id": 0
     },
     {
      "caches": null,
      "cores": null,
      "hugepages": [
       {
        "num_pages": 1,
        "page_size": 2048
       },
       {
        "num_pages": 1,
        "page_size": 1048576
       }
      ],
      "memory": 33604804608,
      "node_id": 1
     }
    ]
    `
	assert.JSONEq(t, expectedTopology, string(topologyJSON))
}
