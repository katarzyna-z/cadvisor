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

	info "github.com/google/cadvisor/info/v1"
	"github.com/google/cadvisor/stats"

	"k8s.io/klog"
)

const (
	cgroupProcfs = "cgroup.procs"
)

var (
	smapsFilePathPattern     = "/proc/%s/smaps"
	clearRefsFilePathPattern = "/proc/%s/clear_refs"

	referencedRegexp = regexp.MustCompile(`Referenced:\s*([0-9]+)\s*kB`)
	isDigitRegExp    = regexp.MustCompile("\\d+")
)

type manager struct {
	resetInterval uint64

	stats.NoopDestroy
}

// NewManager if wss_reset_internal parameter has appropriate value returs new manger of wss metric
// otherwise returns NoopManger
func NewManager(resetInterval uint64, wssEnabled bool) stats.Manager {
	if resetInterval == uint64(0) {
		klog.Warningf("Incorrect value of wss_reset_interval, currently set to %d, working set size metric cannot be provided", resetInterval)
		return &stats.NoopManager{}
	} else if !wssEnabled {
		klog.V(3).Infof("Working set size metric is disabled")
		return &stats.NoopManager{}
	}
	return &manager{resetInterval: resetInterval}
}

// GetCollector returns collector of wss metric
func (m *manager) GetCollector(cgroupPath string) (stats.Collector, error) {
	collector := newCollector(cgroupPath, m.resetInterval)
	return collector, nil
}

func newCollector(cgroupPath string, resetInterval uint64) stats.Collector {
	cgroupCPUPath := filepath.Join(cgroupPath, cgroupProcfs)
	_, err := os.Stat(cgroupCPUPath)
	if err != nil {
		klog.Warningf("Working set size metric is not available for %s cgroup, err: %s", cgroupPath, err)
		return &stats.NoopCollector{}
	}

	collector := &collector{cgroupCPUPath: cgroupCPUPath, resetInterval: resetInterval}
	return collector
}

// collector holds information necessary to calculate working set size
type collector struct {
	// cgroupCPUPath CPU cgroup path
	cgroupCPUPath string
	// Cycles counter for measurements cycles
	cycles uint64
	// resetInterval number of measurement cycles after which referenced bytes are cleared
	resetInterval uint64

	stats.NoopDestroy
}

// UpdateStats calculates working set size and clear referenced bytes
// see: https://github.com/brendangregg/wss#wsspl-referenced-page-flag
func (c *collector) UpdateStats(stats *info.ContainerStats) error {
	c.cycles++

	pids, err := c.getPids()
	if err != nil {
		return err
	}

	referencedKBytes, err := c.getReferenced(pids)
	if err != nil {
		return err
	}

	err = c.clearReferenced(pids)
	if err != nil {
		return err
	}
	stats.Wss = referencedKBytes * 1024
	return nil
}

func (c collector) getPids() ([]string, error) {
	file, err := os.Open(c.cgroupCPUPath)
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
		klog.V(3).Infof("Not found any PID for %s cgroup", c.cgroupCPUPath)
	}
	return pids, nil
}

func (c collector) getReferenced(pids []string) (uint64, error) {
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
			klog.Warningf("Cannot read smaps files for any PID from %s", c.cgroupCPUPath)
		} else if !foundMatch {
			klog.Warningf("Not found any information about referenced bytes in smaps files for any PID from %s", c.cgroupCPUPath)
		}
	}
	return referencedKBytes, nil
}

func (c collector) clearReferenced(pids []string) error {
	if c.resetInterval == 0 {
		return fmt.Errorf("Incorrect of reset interval for wss, ResetInterval: %d", c.resetInterval)
	}

	if c.cycles%c.resetInterval == 0 {
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
