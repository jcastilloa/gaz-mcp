//go:build linux

package diagnostics

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func collectProcessMetrics(procRoot string) map[string]any {
	metrics := map[string]any{}
	readProcessUptime(metrics, filepath.Join(procRoot, "uptime"), filepath.Join(procRoot, "self", "stat"))
	readProcessMemory(metrics, filepath.Join(procRoot, "self", "statm"))
	readProcessIO(metrics, filepath.Join(procRoot, "self", "io"))
	readProcessThreads(metrics, filepath.Join(procRoot, "self", "status"))
	readProcessFDs(metrics, filepath.Join(procRoot, "self", "fd"))
	readProcessRusage(metrics)
	readProcessLimits(metrics)
	return metrics
}

func readProcessUptime(metrics map[string]any, systemUptimeFile, processStatFile string) {
	uptimeFields := strings.Fields(readTextFile(systemUptimeFile))
	if len(uptimeFields) == 0 {
		return
	}
	systemUptime, err := strconv.ParseFloat(uptimeFields[0], 64)
	if err != nil {
		return
	}
	stat := readTextFile(processStatFile)
	commandEnd := strings.LastIndex(stat, ")")
	if commandEnd < 0 {
		return
	}
	fields := strings.Fields(stat[commandEnd+1:])
	const startTimeIndexAfterCommand = 19
	if len(fields) <= startTimeIndexAfterCommand {
		return
	}
	startTicks, ok := parseUint(fields[startTimeIndexAfterCommand])
	if !ok {
		return
	}
	// Linux exposes USER_HZ as 100 on supported architectures.
	const clockTicksPerSecond = 100
	uptime := systemUptime - float64(startTicks)/clockTicksPerSecond
	if uptime >= 0 && !math.IsInf(uptime, 0) && !math.IsNaN(uptime) {
		metrics["uptime_seconds"] = uptime
	}
}

func readProcessMemory(metrics map[string]any, name string) {
	fields := strings.Fields(readTextFile(name))
	if len(fields) < 2 {
		return
	}
	pageSize := uint64(os.Getpagesize())
	if value, ok := parseUint(fields[0]); ok {
		metrics["virtual_memory_bytes"] = value * pageSize
	}
	if value, ok := parseUint(fields[1]); ok {
		metrics["rss_bytes"] = value * pageSize
	}
}

func readProcessIO(metrics map[string]any, name string) {
	values := parseKeyValues(readTextFile(name))
	if value, ok := values["read_bytes"]; ok {
		metrics["io_read_bytes"] = value
	}
	if value, ok := values["write_bytes"]; ok {
		metrics["io_write_bytes"] = value
	}
}

func readProcessThreads(metrics map[string]any, name string) {
	values := parseKeyValues(readTextFile(name))
	if value, ok := values["Threads"]; ok {
		metrics["os_threads"] = value
	}
}

func readProcessFDs(metrics map[string]any, directory string) {
	directoryFile, err := os.Open(directory)
	if err != nil {
		return
	}
	defer directoryFile.Close()
	entries, err := directoryFile.Readdirnames(-1)
	if err != nil {
		return
	}
	count := uint64(len(entries))
	self := strconv.FormatUint(uint64(directoryFile.Fd()), 10)
	for _, entry := range entries {
		if entry != self {
			continue
		}
		info, statErr := os.Lstat(filepath.Join(directory, entry))
		if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			count--
		}
		break
	}
	metrics["open_fds"] = count
}

func readProcessRusage(metrics map[string]any) {
	var usage syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &usage) != nil {
		return
	}
	metrics["cpu_user_seconds"] = timevalSeconds(usage.Utime)
	metrics["cpu_system_seconds"] = timevalSeconds(usage.Stime)
}

func readProcessLimits(metrics map[string]any) {
	var limit syscall.Rlimit
	if syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit) != nil {
		return
	}
	metrics["fd_soft_limit"] = limit.Cur
	metrics["fd_hard_limit"] = limit.Max
}

func timevalSeconds(value syscall.Timeval) float64 {
	return float64(value.Sec) + float64(value.Usec)/1_000_000
}

func readTextFile(name string) string {
	data, err := os.ReadFile(name)
	if err != nil {
		return ""
	}
	return string(data)
}

func parseKeyValues(data string) map[string]uint64 {
	values := make(map[string]uint64)
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if value, ok := parseUint(fields[1]); ok {
			values[strings.TrimSuffix(fields[0], ":")] = value
		}
	}
	return values
}

func parseUint(value string) (uint64, bool) {
	number, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	return number, err == nil
}
