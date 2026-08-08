package diagnostics

import (
	"time"

	domain "github.com/jcastillo/gaz-mcp/mcp/domain/diagnostics"
)

const (
	FindingDBPoolExhausted         = "DB_POOL_EXHAUSTED"
	FindingHTTPConnectionFastHTTP  = "HTTP_CONNECTION_FASTHTTP"
	FindingHTTPConnectionNetHTTP   = "HTTP_CONNECTION_NET_HTTP"
	FindingLockContention          = "LOCK_CONTENTION"
	FindingPossibleGoroutineLeak   = "POSSIBLE_GOROUTINE_LEAK"
	FindingFDLimitPressure         = "FD_LIMIT_PRESSURE"
	FindingPossibleFDLeak          = "POSSIBLE_FD_LEAK"
	FindingHostMemoryPressure      = "HOST_MEMORY_PRESSURE"
	FindingHostCPUPressure         = "HOST_CPU_PRESSURE"
	FindingSystemFileTablePressure = "SYSTEM_FILE_TABLE_PRESSURE"
	FindingCgroupMemoryPressure    = "CGROUP_MEMORY_PRESSURE"
	FindingMemoryLimitMismatch     = "GO_MEMORY_LIMIT_MISMATCH"
	FindingNativeMemoryPressure    = "NATIVE_MEMORY_PRESSURE"
	FindingGCPressure              = "GC_PRESSURE"
	FindingCPUThrottled            = "CPU_THROTTLED"
	FindingSchedulerPressure       = "SCHEDULER_PRESSURE"
	FindingMutexContention         = "MUTEX_CONTENTION"
	FindingPIDPressure             = "PID_LIMIT_PRESSURE"

	ConfidenceConfirmed = "confirmed"
	ConfidenceSuspected = "suspected"
)

type GoroutineReport struct {
	Total  int                      `json:"total"`
	Groups []GoroutineGroup         `json:"groups"`
	Alerts []domain.InspectionAlert `json:"alerts,omitempty"`
}

type GoroutineGroup struct {
	Count      int           `json:"count"`
	State      string        `json:"state"`
	BlockedFor time.Duration `json:"blocked_for_ns,omitempty"`
	Stack      []string      `json:"stack"`
	Findings   []Finding     `json:"findings,omitempty"`
}

type Finding struct {
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Hint        string `json:"hint,omitempty"`
}

type RuntimeStats struct {
	Available       bool                     `json:"available"`
	GoroutineSource string                   `json:"goroutine_source,omitempty"`
	Goroutines      int                      `json:"goroutines,omitempty"`
	Threads         int                      `json:"threads,omitempty"`
	HeapAlloc       uint64                   `json:"heap_alloc_bytes,omitempty"`
	HeapSys         uint64                   `json:"heap_sys_bytes,omitempty"`
	NumGC           uint32                   `json:"num_gc,omitempty"`
	PauseTotal      time.Duration            `json:"pause_total_ns,omitempty"`
	LastGC          time.Time                `json:"last_gc,omitempty"`
	Instrumentation string                   `json:"instrumentation,omitempty"`
	Alerts          []domain.InspectionAlert `json:"alerts,omitempty"`
}

type DBPoolStats struct {
	Name               string        `json:"name"`
	OpenConnections    int           `json:"open_connections"`
	InUse              int           `json:"in_use"`
	Idle               int           `json:"idle"`
	MaxOpenConnections int           `json:"max_open_connections"`
	WaitCount          int64         `json:"wait_count"`
	WaitDuration       time.Duration `json:"wait_duration_ns"`
	MaxIdleClosed      int64         `json:"max_idle_closed,omitempty"`
	MaxIdleTimeClosed  int64         `json:"max_idle_time_closed,omitempty"`
	MaxLifetimeClosed  int64         `json:"max_lifetime_closed,omitempty"`
}

type DiagnosisReport struct {
	Metrics    MetricWindow             `json:"metrics"`
	Goroutines GoroutineReport          `json:"goroutines"`
	Suspicions []Suspicion              `json:"suspicions"`
	Alerts     []domain.InspectionAlert `json:"alerts,omitempty"`
}

type MetricWindow struct {
	Before     MetricSnapshot `json:"before"`
	After      MetricSnapshot `json:"after"`
	DurationNs int64          `json:"duration_ns"`
}

type MetricSnapshot struct {
	ObservedAt  time.Time      `json:"observed_at,omitzero"`
	CollectedAt time.Time      `json:"collected_at,omitzero"`
	Runtime     map[string]any `json:"runtime"`
	Process     map[string]any `json:"process"`
	Host        map[string]any `json:"host"`
	Cgroup      map[string]any `json:"cgroup"`
	DBPools     []DBPoolStats  `json:"db_pools"`
	Custom      map[string]any `json:"custom"`
}

type Suspicion struct {
	Code       string           `json:"code"`
	Severity   string           `json:"severity"`
	Confidence string           `json:"confidence"`
	Reason     string           `json:"reason"`
	Evidence   []MetricEvidence `json:"evidence"`
	Verify     string           `json:"verify"`
}

type MetricEvidence struct {
	Metric   string `json:"metric"`
	Value    any    `json:"value"`
	Previous any    `json:"previous,omitempty"`
}

type ProfileAnalysis struct {
	Profile        domain.ProfileMetadata `json:"profile"`
	SampleType     string                 `json:"sample_type"`
	Unit           string                 `json:"unit"`
	DurationNs     int64                  `json:"duration_ns,omitempty"`
	RatesAvailable bool                   `json:"rates_available"`
	Total          int64                  `json:"total"`
	TotalPerSecond float64                `json:"total_per_second,omitempty"`
	Functions      []FunctionSample       `json:"functions"`
}

type FunctionSample struct {
	Name                string  `json:"name"`
	Flat                int64   `json:"flat"`
	Cumulative          int64   `json:"cumulative"`
	FlatPercent         float64 `json:"flat_percent,omitempty"`
	CumulativePercent   float64 `json:"cumulative_percent,omitempty"`
	FlatPerSecond       float64 `json:"flat_per_second,omitempty"`
	CumulativePerSecond float64 `json:"cumulative_per_second,omitempty"`
}

type ProfileDiff struct {
	ProfileA       domain.ProfileMetadata `json:"profile_a"`
	ProfileB       domain.ProfileMetadata `json:"profile_b"`
	SampleType     string                 `json:"sample_type"`
	Unit           string                 `json:"unit"`
	DurationANs    int64                  `json:"duration_a_ns,omitempty"`
	DurationBNs    int64                  `json:"duration_b_ns,omitempty"`
	RatesAvailable bool                   `json:"rates_available"`
	Functions      []FunctionDelta        `json:"functions"`
}

type FunctionDelta struct {
	Name                string  `json:"name"`
	Flat                int64   `json:"flat_delta"`
	Cumulative          int64   `json:"cumulative_delta"`
	FlatPerSecond       float64 `json:"flat_per_second_delta"`
	CumulativePerSecond float64 `json:"cumulative_per_second_delta"`
}
