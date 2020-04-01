// Copyright 2020 Google Inc. All Rights Reserved.
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

package wss

import (
	"io/ioutil"
	"testing"

	info "github.com/google/cadvisor/info/v1"
	"github.com/stretchr/testify/assert"
)

func TestWss(t *testing.T) {
	//overwrite package variables
	smapsFilePathPattern = "testdata/smaps%s"
	clearRefsFilePathPattern = "testdata/clear_refs%s"

	collector := newCollector("testdata", 3)

	stats := info.ContainerStats{}
	err := collector.UpdateStats(&stats)
	assert.Nil(t, err)
	assert.Equal(t, uint64(416*1024), stats.Wss)

	clearRefsFiles := []string{
		"testdata/clear_refs4",
		"testdata/clear_refs6",
		"testdata/clear_refs8"}

	//check if clear_refs files have proper values
	assert.Equal(t, "0\n", getFileContent(t, clearRefsFiles[0]))
	assert.Equal(t, "0\n", getFileContent(t, clearRefsFiles[1]))
	assert.Equal(t, "0\n", getFileContent(t, clearRefsFiles[2]))
}

func TestWssWhenResetIsNeeded(t *testing.T) {
	//overwrite package variables
	smapsFilePathPattern = "testdata/smaps%s"
	clearRefsFilePathPattern = "testdata/clear_refs%s"

	collector := newCollector("testdata", 1)

	stats := info.ContainerStats{}
	err := collector.UpdateStats(&stats)
	assert.Nil(t, err)
	assert.Equal(t, uint64(416*1024), stats.Wss)

	clearRefsFiles := []string{
		"testdata/clear_refs4",
		"testdata/clear_refs6",
		"testdata/clear_refs8"}

	//check if clear_refs files have proper values
	assert.Equal(t, "1\n", getFileContent(t, clearRefsFiles[0]))
	assert.Equal(t, "1\n", getFileContent(t, clearRefsFiles[1]))
	assert.Equal(t, "1\n", getFileContent(t, clearRefsFiles[2]))

	clearTestData(t, clearRefsFiles)
}

func TestWssGetReferencedWhenSmapsMissing(t *testing.T) {
	//overwrite package variable
	smapsFilePathPattern = "testdata/smaps%s"
	collector := &collector{cgroupCPUPath: "testdata/cgroup.procs", resetInterval: 3}
	pids := []string{"10"}
	referencedKBytes, err := collector.getReferenced(pids)
	assert.Nil(t, err)
	assert.Equal(t, uint64(0), referencedKBytes)
}

func TestWssClearReferencedWhenClearRefsMissing(t *testing.T) {
	//overwrite package variable
	clearRefsFilePathPattern = "testdata/clear_refs%s"

	collector := &collector{cgroupCPUPath: "testdata/cgroup.procs", resetInterval: 1}
	pids := []string{"10"}
	err := collector.clearReferenced(pids)
	assert.Nil(t, err)
}

func getFileContent(t *testing.T, filePath string) string {
	fileContent, err := ioutil.ReadFile(filePath)
	assert.Nil(t, err)
	return string(fileContent)
}

func clearTestData(t *testing.T, clearRefsPaths []string) {
	for _, clearRefsPath := range clearRefsPaths {
		err := ioutil.WriteFile(clearRefsPath, []byte("0\n"), 0644)
		assert.Nil(t, err)
	}
}
