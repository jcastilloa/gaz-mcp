package diagnostics

import (
	"strings"
	"testing"
	"time"
)

const blockedRequests = `goroutine 17 [semacquire, 4 minutes]:
sync.runtime_Semacquire(0xc0001123c0?)
	/usr/local/go/src/runtime/sema.go:71 +0x25
sync.(*WaitGroup).Wait(0xc0001d2300?)
	/usr/local/go/src/sync/waitgroup.go:118 +0x48
database/sql.(*DB).conn(0xc0001d2300, {0x123, 0x456}, 0x1)
	/usr/local/go/src/database/sql/sql.go:1361 +0x425
database/sql.(*DB).QueryContext(0xc0001d2300, {0x123, 0x456}, {0xabcd, 0x12}, {0x0, 0x0, 0x0})
	/usr/local/go/src/database/sql/sql.go:1733 +0xc5
github.com/valyala/fasthttp.(*Server).serveConn(0xc00012c000, {0x789, 0xc000110040})
	/go/pkg/mod/github.com/valyala/fasthttp@v1.55.0/server.go:2378 +0x1283

goroutine 18 [semacquire, 4 minutes]:
sync.runtime_Semacquire(0xc0001123c0?)
	/usr/local/go/src/runtime/sema.go:71 +0x25
sync.(*WaitGroup).Wait(0xc0001d2300?)
	/usr/local/go/src/sync/waitgroup.go:118 +0x48
database/sql.(*DB).conn(0xc0001d2300, {0x123, 0x456}, 0x1)
	/usr/local/go/src/database/sql/sql.go:1361 +0x425
database/sql.(*DB).QueryContext(0xc0001d2300, {0x123, 0x456}, {0xabcd, 0x12}, {0x0, 0x0, 0x0})
	/usr/local/go/src/database/sql/sql.go:1733 +0xc5
github.com/valyala/fasthttp.(*Server).serveConn(0xc00012c000, {0x789, 0xc000110040})
	/go/pkg/mod/github.com/valyala/fasthttp@v1.55.0/server.go:2378 +0x1283

goroutine 19 [chan receive, 6 minutes]:
main.worker()
	/srv/main.go:42 +0x50
`

func TestAnalyzeGoroutinesGroupsEquivalentStacksAndDetectsDBPoolExhaustion(t *testing.T) {
	report, err := AnalyzeGoroutines(blockedRequests)
	if err != nil {
		t.Fatalf("analyze goroutines: %v", err)
	}

	if report.Total != 3 {
		t.Fatalf("total = %d, want 3", report.Total)
	}
	if len(report.Groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(report.Groups))
	}

	group := report.Groups[0]
	if group.Count != 2 {
		t.Fatalf("group count = %d, want 2", group.Count)
	}
	if group.BlockedFor != 4*time.Minute {
		t.Fatalf("blocked for = %s, want 4m", group.BlockedFor)
	}
	if !hasFinding(group.Findings, FindingDBPoolExhausted) {
		t.Fatalf("findings = %#v, want %q", group.Findings, FindingDBPoolExhausted)
	}
	if !hasFinding(group.Findings, FindingHTTPInflightFastHTTP) {
		t.Fatalf("findings = %#v, want %q", group.Findings, FindingHTTPInflightFastHTTP)
	}
}

func TestDiagnoseConfirmsDBPoolExhaustionWithGoroutinesAndPoolStats(t *testing.T) {
	goroutines, err := AnalyzeGoroutines(blockedRequests)
	if err != nil {
		t.Fatalf("analyze goroutines: %v", err)
	}

	report := BuildDiagnosis(RuntimeStats{Goroutines: 1_240}, goroutines, []DBPoolStats{{
		Name:               "primary",
		OpenConnections:    100,
		InUse:              100,
		Idle:               0,
		MaxOpenConnections: 100,
		WaitCount:          15_340,
		WaitDuration:       8*time.Minute + 32*time.Second,
	}})

	if len(report.Conclusions) == 0 {
		t.Fatal("expected a diagnosis conclusion")
	}
	if report.Conclusions[0].Code != FindingDBPoolExhausted {
		t.Fatalf("conclusion = %q, want %q", report.Conclusions[0].Code, FindingDBPoolExhausted)
	}
	if !strings.Contains(report.Conclusions[0].Evidence, "primary") {
		t.Fatalf("evidence = %q, want pool name", report.Conclusions[0].Evidence)
	}
}

func TestDiagnoseDoesNotConfirmPoolExhaustionWithoutPoolMetrics(t *testing.T) {
	goroutines, err := AnalyzeGoroutines(blockedRequests)
	if err != nil {
		t.Fatalf("analyze goroutines: %v", err)
	}

	report := BuildDiagnosis(RuntimeStats{}, goroutines, nil)
	if len(report.Conclusions) != 1 {
		t.Fatalf("conclusions = %#v, want one suspected conclusion", report.Conclusions)
	}
	if report.Conclusions[0].Confidence != ConfidenceSuspected {
		t.Fatalf("confidence = %q, want %q", report.Conclusions[0].Confidence, ConfidenceSuspected)
	}
}

func TestDiagnoseDoesNotSuspectPoolExhaustionWhenExposedPoolsHaveNoWaits(t *testing.T) {
	goroutines, err := AnalyzeGoroutines(blockedRequests)
	if err != nil {
		t.Fatalf("analyze goroutines: %v", err)
	}

	report := BuildDiagnosis(RuntimeStats{}, goroutines, []DBPoolStats{
		{Name: "database", MaxOpenConnections: 50},
		{Name: "database_comments", MaxOpenConnections: 50},
	})
	if len(report.Conclusions) != 0 {
		t.Fatalf("conclusions = %#v, want no conclusion when exposed pools have no waits", report.Conclusions)
	}
}

func TestDiagnoseSuspectsPoolExhaustionWhenExposedPoolsHaveWaitsWithoutSaturation(t *testing.T) {
	goroutines, err := AnalyzeGoroutines(blockedRequests)
	if err != nil {
		t.Fatalf("analyze goroutines: %v", err)
	}

	report := BuildDiagnosis(RuntimeStats{}, goroutines, []DBPoolStats{{
		Name:               "database",
		OpenConnections:    4,
		InUse:              2,
		Idle:               2,
		MaxOpenConnections: 50,
		WaitCount:          1,
	}})
	if len(report.Conclusions) != 1 || report.Conclusions[0].Confidence != ConfidenceSuspected {
		t.Fatalf("conclusions = %#v, want one suspected conclusion", report.Conclusions)
	}
}

func TestAnalyzeGoroutinesDoesNotTreatConnectionOpenerAsPoolWaiter(t *testing.T) {
	report, err := AnalyzeGoroutines(`goroutine 31 [select]:
database/sql.(*DB).connectionOpener(0xc0001d2300, {0x123, 0x456})
	/usr/local/go/src/database/sql/sql.go:1253 +0x87
`)
	if err != nil {
		t.Fatalf("analyze goroutines: %v", err)
	}
	if hasFinding(report.Groups[0].Findings, FindingDBPoolExhausted) {
		t.Fatalf("findings = %#v, connectionOpener is a normal database/sql worker", report.Groups[0].Findings)
	}
}

func hasFinding(findings []Finding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
