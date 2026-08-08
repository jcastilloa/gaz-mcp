//go:build linux

package diagnostics

import (
	"math"
	"path/filepath"
	"strconv"
	"strings"
)

func collectCgroupMetrics(procRoot, cgroupRoot string) map[string]any {
	group, ok := cgroupV2Path(readTextFile(filepath.Join(procRoot, "self", "cgroup")))
	if !ok {
		return map[string]any{}
	}
	directory := filepath.Join(cgroupRoot, filepath.FromSlash(strings.TrimPrefix(group, "/")))
	metrics := make(map[string]any)
	readCgroupNumber(metrics, directory, "memory.current", "memory_current_bytes")
	readCgroupLimit(metrics, directory, "memory.max", "memory_max_bytes")
	readCgroupNumber(metrics, directory, "pids.current", "pids_current")
	readCgroupLimit(metrics, directory, "pids.max", "pids_max")
	readCgroupEvents(metrics, directory)
	readCgroupCPU(metrics, directory)
	readPressure(metrics, directory, "cpu")
	readPressure(metrics, directory, "memory")
	readPressure(metrics, directory, "io")
	return metrics
}

func cgroupV2Path(data string) (string, bool) {
	for _, line := range strings.Split(data, "\n") {
		if path, found := strings.CutPrefix(line, "0::"); found {
			return strings.TrimSpace(path), true
		}
	}
	return "", false
}

func readCgroupNumber(metrics map[string]any, directory, file, name string) {
	value := strings.TrimSpace(readTextFile(filepath.Join(directory, file)))
	if number, ok := parseUint(value); ok {
		metrics[name] = number
	}
}

func readCgroupLimit(metrics map[string]any, directory, file, name string) {
	value := strings.TrimSpace(readTextFile(filepath.Join(directory, file)))
	if value != "max" {
		if number, ok := parseUint(value); ok {
			metrics[name] = number
		}
	}
}

func readCgroupEvents(metrics map[string]any, directory string) {
	values := parseKeyValues(readTextFile(filepath.Join(directory, "memory.events")))
	if len(values) > 0 {
		metrics["memory_events"] = values
	}
}

func readCgroupCPU(metrics map[string]any, directory string) {
	fields := strings.Fields(readTextFile(filepath.Join(directory, "cpu.max")))
	if len(fields) == 2 && fields[0] != "max" {
		quota, quotaOK := parseUint(fields[0])
		period, periodOK := parseUint(fields[1])
		if quotaOK && periodOK && period > 0 {
			metrics["cpu_quota_cores"] = float64(quota) / float64(period)
		}
	}
	values := parseKeyValues(readTextFile(filepath.Join(directory, "cpu.stat")))
	copyCgroupValue(metrics, values, "usage_usec", "cpu_usage_usec")
	copyCgroupValue(metrics, values, "throttled_usec", "cpu_throttled_usec")
	copyCgroupValue(metrics, values, "nr_periods", "cpu_periods")
	copyCgroupValue(metrics, values, "nr_throttled", "cpu_throttled_periods")
}

func copyCgroupValue(metrics map[string]any, values map[string]uint64, source, target string) {
	if value, ok := values[source]; ok {
		metrics[target] = value
	}
}

func readPressure(metrics map[string]any, directory, resource string) {
	values := parsePressure(readTextFile(filepath.Join(directory, resource+".pressure")))
	if len(values) > 0 {
		metrics[resource+"_pressure"] = values
	}
}

func parsePressure(data string) map[string]any {
	result := make(map[string]any)
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		prefix := fields[0] + "_"
		for _, field := range fields[1:] {
			name, value, ok := strings.Cut(field, "=")
			if !ok {
				continue
			}
			if name == "total" {
				if number, valid := parseUint(value); valid {
					result[prefix+"total_usec"] = number
				}
				continue
			}
			number, err := strconv.ParseFloat(value, 64)
			if err == nil && !math.IsNaN(number) && !math.IsInf(number, 0) {
				result[prefix+name] = number
			}
		}
	}
	return result
}
