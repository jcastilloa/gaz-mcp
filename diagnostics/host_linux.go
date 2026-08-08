//go:build linux

package diagnostics

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func collectHostMetrics(procRoot string) map[string]any {
	metrics := map[string]any{"logical_cpus": runtime.NumCPU()}
	readHostMemory(metrics, filepath.Join(procRoot, "meminfo"))
	readHostLoad(metrics, filepath.Join(procRoot, "loadavg"))
	readHostUptime(metrics, filepath.Join(procRoot, "uptime"))
	readHostFileHandles(metrics, filepath.Join(procRoot, "sys", "fs", "file-nr"))
	readHostSwap(metrics, filepath.Join(procRoot, "vmstat"))
	readHostPressure(metrics, filepath.Join(procRoot, "pressure"))
	return metrics
}

func readHostMemory(metrics map[string]any, name string) {
	values := parseKeyValues(readTextFile(name))
	copyKilobytes(metrics, values, "MemTotal", "memory_total_bytes")
	copyKilobytes(metrics, values, "MemAvailable", "memory_available_bytes")
	copyKilobytes(metrics, values, "SwapTotal", "swap_total_bytes")
	copyKilobytes(metrics, values, "SwapFree", "swap_free_bytes")
}

func copyKilobytes(metrics map[string]any, values map[string]uint64, source, target string) {
	if value, ok := values[source]; ok {
		metrics[target] = value << 10
	}
}

func readHostLoad(metrics map[string]any, name string) {
	fields := strings.Fields(readTextFile(name))
	if len(fields) < 4 {
		return
	}
	for index, name := range []string{"load_1", "load_5", "load_15"} {
		if value, err := strconv.ParseFloat(fields[index], 64); err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) {
			metrics[name] = value
		}
	}
	processes := strings.SplitN(fields[3], "/", 2)
	if len(processes) == 2 {
		if value, ok := parseUint(processes[0]); ok {
			metrics["runnable_processes"] = value
		}
		if value, ok := parseUint(processes[1]); ok {
			metrics["total_processes"] = value
		}
	}
}

func readHostUptime(metrics map[string]any, name string) {
	fields := strings.Fields(readTextFile(name))
	if len(fields) == 0 {
		return
	}
	if value, err := strconv.ParseFloat(fields[0], 64); err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) {
		metrics["uptime_seconds"] = value
	}
}

func readHostFileHandles(metrics map[string]any, name string) {
	fields := strings.Fields(readTextFile(name))
	if len(fields) < 3 {
		return
	}
	if value, ok := parseUint(fields[0]); ok {
		metrics["file_handles_allocated"] = value
	}
	if value, ok := parseUint(fields[2]); ok {
		metrics["file_handles_max"] = value
	}
}

func readHostSwap(metrics map[string]any, name string) {
	values := parseKeyValues(readTextFile(name))
	pageSize := uint64(os.Getpagesize())
	if value, ok := values["pswpin"]; ok {
		metrics["swap_in_bytes_total"] = value * pageSize
	}
	if value, ok := values["pswpout"]; ok {
		metrics["swap_out_bytes_total"] = value * pageSize
	}
}

func readHostPressure(metrics map[string]any, directory string) {
	for _, resource := range []string{"cpu", "memory", "io"} {
		values := parsePressure(readTextFile(filepath.Join(directory, resource)))
		if len(values) > 0 {
			metrics[resource+"_pressure"] = values
		}
	}
}
