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
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"

	"k8s.io/klog"
)

const (
	cgroupProcfs = "cgroup.procs"
)

var (
	smapsFilePathPattern     = "/proc/%d/smaps"
	clearRefsFilePathPattern = "/proc/%d/clear_refs"

	referencedRegexp = regexp.MustCompile(`Referenced:\s*([0-9]+)\s*kB`)
	isDigitRegExp    = regexp.MustCompile("\\d+")
)

// GetStat calculates working set size and clear referenced bytes
// see: https://github.com/brendangregg/wss#wsspl-referenced-page-flag
func GetStat(pids []int, cycles uint64, resetInterval uint64) (uint64, error) {
	referencedKBytes, err := getReferenced(pids)
	if err != nil {
		return uint64(0), err
	}

	err = clearReferenced(pids, cycles, resetInterval)
	if err != nil {
		return uint64(0), err
	}
	return referencedKBytes * 1024, nil
}

func getReferenced(pids []int) (uint64, error) {
	referencedKBytes := uint64(0)
	readSmapsContent := false
	foundMatch := false
	for _, pid := range pids {
		smapsFilePath := fmt.Sprintf(smapsFilePathPattern, pid)
		smapsContent, err := ioutil.ReadFile(smapsFilePath)
		if err != nil {
			klog.V(3).Infof("Cannot read %s file, err: %s", smapsFilePath, err)
			if os.IsNotExist(err) {
				continue //smaps file does not exists for all PIDs
			}
			return 0, err
		}
		readSmapsContent = true

		allMatches := referencedRegexp.FindAllSubmatch(smapsContent, -1)
		if len(allMatches) == 0 {
			klog.V(3).Infof("Not found any information about referenced bytes in %s file", smapsFilePath)
			continue // referenced bytes may not exist in smaps file
		}

		for _, matches := range allMatches {
			if len(matches) != 2 {
				return 0, fmt.Errorf("failed to match regexp in output: %s", string(smapsContent))
			}
			foundMatch = true
			referenced, err := strconv.ParseUint(string(matches[1]), 10, 64)
			if err != nil {
				return 0, err
			}
			referencedKBytes += referenced
		}
	}

	if len(pids) != 0 {
		if !readSmapsContent {
			klog.Warningf("Cannot read smaps files for any PID from %s", "CONTAINER")
		} else if !foundMatch {
			klog.Warningf("Not found any information about referenced bytes in smaps files for any PID from %s", "CONTAINER")
		}
	}
	return referencedKBytes, nil
}

func clearReferenced(pids []int, cycles uint64, resetInterval uint64) error {
	if resetInterval == 0 {
		return fmt.Errorf("Incorrect of reset interval for wss, ResetInterval: %d", resetInterval)
	}

	if cycles%resetInterval == 0 {
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
