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
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

const (
	cgroupProcfs = "cgroup.procs"
)

var (
	smapsFilePathPattern     = "/proc/%s/smaps"
	clearRefsFilePathPattern = "/proc/%s/clear_refs"

	referencedRegexp = regexp.MustCompile(`Referenced:\s*([0-9]+) kB`)
	isDigitRegExp    = regexp.MustCompile("\\d+")
)

// Wss holds information necessary to calculate working set size
type Wss struct {
	// CgroupCPUPath CPU cgroup path
	CgroupCPUPath string
	// Cycles counter for measurements cycles
	Cycles uint64
	// ResetInterval number of measurement csycles after which referenced bytes are cleared
	ResetInterval uint64
}

// GetStat calculate working set size and clear referenced bytes
// see:  https://github.com/brendangregg/wss#wsspl-referenced-page-flag
func (w Wss) GetStat() (uint64, error) {
	w.Cycles++

	pids, err := w.getPids()
	if err != nil {
		return 0, err
	}

	referencedKBytes, err := w.getReferenced(pids)
	if err != nil {
		return 0, err
	}

	err = w.clearReferenced(pids)
	if err != nil {
		return 0, err
	}

	return referencedKBytes * 1024, nil
}

func (w Wss) getPids() ([]string, error) {
	file, err := os.Open(filepath.Join(w.CgroupCPUPath, cgroupProcfs))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	pids := make([]string, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		pids = append(pids, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Error in reading PIDs, err: %s", err)
	}

	if len(pids) == 0 {
		return nil, fmt.Errorf("Not found any PID for %s cgroup", w.CgroupCPUPath)
	}
	return pids, nil
}

func (w Wss) getContainerPids(hostPids map[string]interface{}, cgroupPids map[string]interface{}) ([]string, error) {
	pids := make([]string, 0)
	for cgroupPid := range cgroupPids {
		if _, ok := hostPids[cgroupPid]; ok {
			pids = append(pids, cgroupPid)
		}
	}

	if len(pids) == 0 {
		return nil, fmt.Errorf("Container does not have any PID, cgroup PIDs was read from %s", w.CgroupCPUPath)
	}
	return pids, nil
}

func (w Wss) getReferenced(pids []string) (uint64, error) {
	referencedKBytes := uint64(0)
	for _, pid := range pids {
		smapsFilePath := fmt.Sprintf(smapsFilePathPattern, pid)

		smapsContent, err := ioutil.ReadFile(smapsFilePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue //smaps file does not exists for all PIDs
			}
			return 0, err
		}

		allMatches := referencedRegexp.FindAllSubmatch(smapsContent, -1)
		if len(allMatches) == 0 {
			continue // referenced bytes may not exist in smaps file
		}

		for _, matches := range allMatches {
			if len(matches) != 2 {
				return 0, fmt.Errorf("failed to match regexp in output: %s", string(smapsContent))
			}
			referenced, err := strconv.ParseUint(string(matches[1]), 10, 64)
			if err != nil {
				return 0, err
			}
			referencedKBytes += referenced
		}
	}
	return referencedKBytes, nil
}

func (w Wss) clearReferenced(pids []string) error {
	if w.ResetInterval == 0 {
		return fmt.Errorf("Incorrect of reset interval for wss, ResetInterval: %d", w.ResetInterval)
	}

	if w.Cycles%w.ResetInterval == 0 {
		for _, pid := range pids {
			clearRefsFilePath := fmt.Sprintf(clearRefsFilePathPattern, pid)
			clerRefsFile, err := os.OpenFile(clearRefsFilePath, os.O_WRONLY, 0644)
			if err != nil {
				// clear_refs file may not exist for all PIDs
				continue
			}
			_, err = clerRefsFile.WriteString("1\n")
			if err != nil {
				return err
			}
			err = clerRefsFile.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
