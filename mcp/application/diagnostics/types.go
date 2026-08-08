package diagnostics

import (
	"time"

	domain "github.com/jcastillo/gaz-mcp/mcp/domain/diagnostics"
)

const (
	FindingDBPoolExhausted       = "DB_POOL_EXHAUSTED"
	FindingHTTPInflightFastHTTP  = "HTTP_INFLIGHT_FASTHTTP"
	FindingHTTPInflightNetHTTP   = "HTTP_INFLIGHT_NET_HTTP"
	FindingLockContention        = "LOCK_CONTENTION"
	FindingPossibleGoroutineLeak = "POSSIBLE_GOROUTINE_LEAK"

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
	Runtime     RuntimeStats             `json:"runtime"`
	Goroutines  GoroutineReport          `json:"goroutines"`
	DBPools     []DBPoolStats            `json:"db_pools,omitempty"`
	Conclusions []Conclusion             `json:"conclusions,omitempty"`
	Alerts      []domain.InspectionAlert `json:"alerts,omitempty"`
}

type Conclusion struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Confidence string `json:"confidence"`
	Summary    string `json:"summary"`
	Evidence   string `json:"evidence"`
	Hint       string `json:"hint,omitempty"`
}

type ProfileAnalysis struct {
	Profile    domain.ProfileMetadata `json:"profile"`
	SampleType string                 `json:"sample_type"`
	Unit       string                 `json:"unit"`
	Total      int64                  `json:"total"`
	Functions  []FunctionSample       `json:"functions"`
}

type FunctionSample struct {
	Name       string `json:"name"`
	Flat       int64  `json:"flat"`
	Cumulative int64  `json:"cumulative"`
}

type ProfileDiff struct {
	ProfileA   domain.ProfileMetadata `json:"profile_a"`
	ProfileB   domain.ProfileMetadata `json:"profile_b"`
	SampleType string                 `json:"sample_type"`
	Unit       string                 `json:"unit"`
	Functions  []FunctionDelta        `json:"functions"`
}

type FunctionDelta struct {
	Name       string `json:"name"`
	Flat       int64  `json:"flat_delta"`
	Cumulative int64  `json:"cumulative_delta"`
}
