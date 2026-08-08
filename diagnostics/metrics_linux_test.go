//go:build linux

package diagnostics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxProcessMetricsReadProcAndFileDescriptorLimit(t *testing.T) {
	procRoot := t.TempDir()
	self := filepath.Join(procRoot, "self")
	if err := os.MkdirAll(filepath.Join(self, "fd"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(self, "statm"), "100 25 0 0 0 0 0\n")
	writeTestFile(t, filepath.Join(self, "io"), "read_bytes: 4096\nwrite_bytes: 8192\n")
	writeTestFile(t, filepath.Join(procRoot, "uptime"), "1000.00 500.00\n")
	writeTestFile(t, filepath.Join(self, "stat"), "123 (service worker) S 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 5000 0 0\n")
	writeTestFile(t, filepath.Join(self, "fd", "3"), "")
	writeTestFile(t, filepath.Join(self, "fd", "4"), "")

	stats := collectProcessMetrics(procRoot)
	pageSize := uint64(os.Getpagesize())
	assertMetric(t, stats, "virtual_memory_bytes", 100*pageSize)
	assertMetric(t, stats, "rss_bytes", 25*pageSize)
	assertMetric(t, stats, "io_read_bytes", uint64(4096))
	assertMetric(t, stats, "io_write_bytes", uint64(8192))
	assertMetric(t, stats, "uptime_seconds", 950.0)
	assertMetric(t, stats, "open_fds", uint64(2))
	if number, ok := stats["fd_soft_limit"].(uint64); !ok || number == 0 {
		t.Fatalf("fd_soft_limit = %#v, want a positive integer", stats["fd_soft_limit"])
	}
}

func TestLinuxCgroupMetricsReadV2LimitsEventsAndPressure(t *testing.T) {
	procRoot := t.TempDir()
	cgroupRoot := t.TempDir()
	writeTestFile(t, filepath.Join(procRoot, "self", "cgroup"), "0::/service.slice\n")
	group := filepath.Join(cgroupRoot, "service.slice")
	writeTestFile(t, filepath.Join(group, "memory.current"), "900\n")
	writeTestFile(t, filepath.Join(group, "memory.max"), "1000\n")
	writeTestFile(t, filepath.Join(group, "memory.events"), "low 0\nhigh 2\nmax 3\noom 1\noom_kill 1\n")
	writeTestFile(t, filepath.Join(group, "cpu.max"), "200000 100000\n")
	writeTestFile(t, filepath.Join(group, "cpu.stat"), "usage_usec 1000\nnr_periods 50\nnr_throttled 10\nthrottled_usec 250\n")
	writeTestFile(t, filepath.Join(group, "pids.current"), "42\n")
	writeTestFile(t, filepath.Join(group, "pids.max"), "100\n")
	writeTestFile(t, filepath.Join(group, "cpu.pressure"), "some avg10=1.25 avg60=0.50 avg300=0.10 total=123\nfull avg10=0.25 avg60=0.10 avg300=0.01 total=12\n")

	stats := collectCgroupMetrics(procRoot, cgroupRoot)
	assertMetric(t, stats, "memory_current_bytes", uint64(900))
	assertMetric(t, stats, "memory_max_bytes", uint64(1000))
	assertMetric(t, stats, "cpu_quota_cores", 2.0)
	assertMetric(t, stats, "cpu_usage_usec", uint64(1000))
	assertMetric(t, stats, "cpu_throttled_usec", uint64(250))
	assertMetric(t, stats, "cpu_periods", uint64(50))
	assertMetric(t, stats, "cpu_throttled_periods", uint64(10))
	assertMetric(t, stats, "pids_current", uint64(42))
	assertMetric(t, stats, "pids_max", uint64(100))

	events := stats["memory_events"].(map[string]uint64)
	if events["high"] != 2 || events["oom"] != 1 || events["oom_kill"] != 1 {
		t.Fatalf("memory events = %#v", events)
	}
	pressure := stats["cpu_pressure"].(map[string]any)
	if pressure["some_avg10"] != 1.25 || pressure["full_total_usec"] != uint64(12) {
		t.Fatalf("cpu pressure = %#v", pressure)
	}
}

func TestLinuxHostMetricsReadMemoryLoadFileHandlesAndPressure(t *testing.T) {
	procRoot := t.TempDir()
	writeTestFile(t, filepath.Join(procRoot, "meminfo"), "MemTotal: 1000 kB\nMemAvailable: 125 kB\nSwapTotal: 500 kB\nSwapFree: 200 kB\n")
	writeTestFile(t, filepath.Join(procRoot, "loadavg"), "4.50 3.00 2.00 7/200 42\n")
	writeTestFile(t, filepath.Join(procRoot, "uptime"), "1234.50 500.00\n")
	writeTestFile(t, filepath.Join(procRoot, "sys", "fs", "file-nr"), "900 0 1000\n")
	writeTestFile(t, filepath.Join(procRoot, "vmstat"), "pswpin 10\npswpout 20\n")
	writeTestFile(t, filepath.Join(procRoot, "pressure", "cpu"), "some avg10=25.00 avg60=5.00 avg300=1.00 total=123\n")

	stats := collectHostMetrics(procRoot)
	assertMetric(t, stats, "memory_total_bytes", uint64(1000<<10))
	assertMetric(t, stats, "memory_available_bytes", uint64(125<<10))
	assertMetric(t, stats, "swap_total_bytes", uint64(500<<10))
	assertMetric(t, stats, "swap_free_bytes", uint64(200<<10))
	assertMetric(t, stats, "load_1", 4.5)
	assertMetric(t, stats, "load_5", 3.0)
	assertMetric(t, stats, "load_15", 2.0)
	assertMetric(t, stats, "runnable_processes", uint64(7))
	assertMetric(t, stats, "total_processes", uint64(200))
	assertMetric(t, stats, "uptime_seconds", 1234.5)
	assertMetric(t, stats, "file_handles_allocated", uint64(900))
	assertMetric(t, stats, "file_handles_max", uint64(1000))
	assertMetric(t, stats, "swap_in_bytes_total", uint64(10)*uint64(os.Getpagesize()))
	assertMetric(t, stats, "swap_out_bytes_total", uint64(20)*uint64(os.Getpagesize()))

	pressure := stats["cpu_pressure"].(map[string]any)
	if pressure["some_avg10"] != 25.0 {
		t.Fatalf("cpu pressure = %#v", pressure)
	}
}

func TestLinuxCollectorsOmitMalformedMetrics(t *testing.T) {
	procRoot := t.TempDir()
	cgroupRoot := t.TempDir()
	writeTestFile(t, filepath.Join(procRoot, "meminfo"), "MemTotal: invalid kB\nMemAvailable: 125 kB\n")
	writeTestFile(t, filepath.Join(procRoot, "loadavg"), "NaN 3.00 2.00 invalid/200 42\n")
	writeTestFile(t, filepath.Join(procRoot, "self", "statm"), "invalid 25 0 0 0 0 0\n")
	writeTestFile(t, filepath.Join(procRoot, "self", "cgroup"), "0::/service.slice\n")
	group := filepath.Join(cgroupRoot, "service.slice")
	writeTestFile(t, filepath.Join(group, "memory.current"), "invalid\n")
	writeTestFile(t, filepath.Join(group, "memory.max"), "1000\n")
	writeTestFile(t, filepath.Join(group, "cpu.max"), "invalid 100000\n")
	writeTestFile(t, filepath.Join(group, "cpu.pressure"), "some avg10=NaN total=invalid\n")

	host := collectHostMetrics(procRoot)
	for _, name := range []string{"memory_total_bytes", "load_1", "runnable_processes"} {
		if _, exists := host[name]; exists {
			t.Errorf("host metric %q must be omitted when malformed: %#v", name, host[name])
		}
	}
	process := collectProcessMetrics(procRoot)
	if _, exists := process["virtual_memory_bytes"]; exists {
		t.Errorf("virtual_memory_bytes must be omitted when malformed: %#v", process["virtual_memory_bytes"])
	}
	cgroup := collectCgroupMetrics(procRoot, cgroupRoot)
	for _, name := range []string{"memory_current_bytes", "cpu_quota_cores"} {
		if _, exists := cgroup[name]; exists {
			t.Errorf("cgroup metric %q must be omitted when malformed: %#v", name, cgroup[name])
		}
	}
	if pressure, exists := cgroup["cpu_pressure"].(map[string]any); exists {
		if _, exists := pressure["some_avg10"]; exists {
			t.Errorf("non-finite pressure average must be omitted: %#v", pressure)
		}
		if _, exists := pressure["some_total_usec"]; exists {
			t.Errorf("malformed pressure total must be omitted: %#v", pressure)
		}
	}
}

func writeTestFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertMetric(t *testing.T, stats map[string]any, name string, expected any) {
	t.Helper()
	if stats[name] != expected {
		t.Fatalf("%s = %#v, want %#v", name, stats[name], expected)
	}
}
