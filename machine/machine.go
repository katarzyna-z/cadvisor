// Copyright 2015 Google Inc. All Rights Reserved.
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

// The machine package contains functions that extract machine-level specs.
package machine

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"

	// s390/s390x changes
	"runtime"

	info "github.com/google/cadvisor/info/v1"
	"github.com/google/cadvisor/utils"
	"github.com/google/cadvisor/utils/sysfs"
	"github.com/google/cadvisor/utils/sysinfo"

	"golang.org/x/sys/unix"
	"k8s.io/klog"
)

var (
	cpuRegExp     = regexp.MustCompile(`^processor\s*:\s*([0-9]+)$`)
	coreRegExp    = regexp.MustCompile(`^core id\s*:\s*([0-9]+)$`)
	nodeRegExp    = regexp.MustCompile(`^physical id\s*:\s*([0-9]+)$`)
	nodeBusRegExp = regexp.MustCompile(`^node([0-9]+)$`)
	// Power systems have a different format so cater for both
	cpuClockSpeedMHz     = regexp.MustCompile(`(?:cpu MHz|clock)\s*:\s*([0-9]+\.[0-9]+)(?:MHz)?`)
	memoryCapacityRegexp = regexp.MustCompile(`MemTotal:\s*([0-9]+) kB`)
	swapCapacityRegexp   = regexp.MustCompile(`SwapTotal:\s*([0-9]+) kB`)
	machineArch = getMachineArch()
)

const maxFreqFile = "/sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_max_freq"
const cpuBusPath = "/sys/bus/cpu/devices/"
const nodePath = "/sys/devices/system/node"

// GetClockSpeed returns the CPU clock speed, given a []byte formatted as the /proc/cpuinfo file.
func GetClockSpeed(procInfo []byte) (uint64, error) {
	// s390/s390x, mips64, riscv64, aarch64 and arm32 changes
	if isMips64() || isSystemZ() || isAArch64() || isArm32() || isRiscv64() {
		return 0, nil
	}

	// First look through sys to find a max supported cpu frequency.
	if utils.FileExists(maxFreqFile) {
		val, err := ioutil.ReadFile(maxFreqFile)
		if err != nil {
			return 0, err
		}
		var maxFreq uint64
		n, err := fmt.Sscanf(string(val), "%d", &maxFreq)
		if err != nil || n != 1 {
			return 0, fmt.Errorf("could not parse frequency %q", val)
		}
		return maxFreq, nil
	}
	// Fall back to /proc/cpuinfo
	matches := cpuClockSpeedMHz.FindSubmatch(procInfo)
	if len(matches) != 2 {
		return 0, fmt.Errorf("could not detect clock speed from output: %q", string(procInfo))
	}

	speed, err := strconv.ParseFloat(string(matches[1]), 64)
	if err != nil {
		return 0, err
	}
	// Convert to kHz
	return uint64(speed * 1000), nil
}

// GetMachineMemoryCapacity returns the machine's total memory from /proc/meminfo.
// Returns the total memory capacity as an uint64 (number of bytes).
func GetMachineMemoryCapacity() (uint64, error) {
	out, err := ioutil.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, err
	}

	memoryCapacity, err := parseCapacity(out, memoryCapacityRegexp)
	if err != nil {
		return 0, err
	}
	return memoryCapacity, err
}

// GetMachineSwapCapacity returns the machine's total swap from /proc/meminfo.
// Returns the total swap capacity as an uint64 (number of bytes).
func GetMachineSwapCapacity() (uint64, error) {
	out, err := ioutil.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, err
	}

	swapCapacity, err := parseCapacity(out, swapCapacityRegexp)
	if err != nil {
		return 0, err
	}
	return swapCapacity, err
}

// GetTopology returns CPU topology reading information from sysfs
func GetTopology(sysFs sysfs.SysFs) ([]info.Node, int, error) {
	// s390/s390x changes
	if isSystemZ() {
		return nil, getNumCores(), nil
	}
	return sysinfo.GetNodesInfo(sysFs)
}

// parseCapacity matches a Regexp in a []byte, returning the resulting value in bytes.
// Assumes that the value matched by the Regexp is in KB.
func parseCapacity(b []byte, r *regexp.Regexp) (uint64, error) {
	matches := r.FindSubmatch(b)
	if len(matches) != 2 {
		return 0, fmt.Errorf("failed to match regexp in output: %q", string(b))
	}
	m, err := strconv.ParseUint(string(matches[1]), 10, 64)
	if err != nil {
		return 0, err
	}

	// Convert to bytes.
	return m * 1024, err
}

func extractValue(s string, r *regexp.Regexp) (bool, int, error) {
	matches := r.FindSubmatch([]byte(s))
	if len(matches) == 2 {
		val, err := strconv.ParseInt(string(matches[1]), 10, 32)
		if err != nil {
			return false, -1, err
		}
		return true, int(val), nil
	}
	return false, -1, nil
}

// s390/s390x changes
func getMachineArch() string {
	uname := unix.Utsname{}
	err := unix.Uname(&uname)
	if err != nil {
		klog.Errorf("Cannot get machine architecture, err: %v", err)
		return ""
	}
	return string(uname.Machine[:])
}

// arm32 chanes
func isArm32() bool {
	return strings.Contains(machineArch, "arm")
}

// aarch64 changes
func isAArch64() bool {
	return strings.Contains(machineArch, "aarch64")
}

// s390/s390x changes
func isSystemZ() bool {
	return strings.Contains(machineArch, "390")
}

// riscv64 changes
func isRiscv64() bool {
	return strings.Contains(machineArch, "riscv64")
}

// mips64 changes
func isMips64() bool {
	return strings.Contains(machineArch, "mips64")
}

// s390/s390x changes
func getNumCores() int {
	maxProcs := runtime.GOMAXPROCS(0)
	numCPU := runtime.NumCPU()

	if maxProcs < numCPU {
		return maxProcs
	}

	return numCPU
}
