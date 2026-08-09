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
	if report.BlocksParsed != 3 || report.BlocksDiscarded != 0 || report.Completeness != GoroutineCompletenessComplete {
		t.Fatalf("parse metadata = %#v, want complete three-block dump", report)
	}

	group := report.Groups[0]
	if group.Fingerprint == "" {
		t.Fatalf("group = %#v, want stable fingerprint", group)
	}
	if group.Count != 2 {
		t.Fatalf("group count = %d, want 2", group.Count)
	}
	if group.BlockedFor != 4*time.Minute {
		t.Fatalf("blocked for = %s, want 4m", group.BlockedFor)
	}
	if !hasFinding(group.Findings, FindingDBPoolExhausted) {
		t.Fatalf("findings = %#v, want %q", group.Findings, FindingDBPoolExhausted)
	}
	if !hasFinding(group.Findings, FindingHTTPConnectionFastHTTP) {
		t.Fatalf("findings = %#v, want %q", group.Findings, FindingHTTPConnectionFastHTTP)
	}
}

func TestAnalyzeGoroutinesReportsDiscardedBlocks(t *testing.T) {
	report, err := AnalyzeGoroutines("not a goroutine\n\n" + blockedRequests)
	if err != nil {
		t.Fatalf("analyze goroutines: %v", err)
	}
	if report.BlocksParsed != 3 || report.BlocksDiscarded != 1 || report.Completeness != GoroutineCompletenessPartial {
		t.Fatalf("parse metadata = %#v, want one discarded block", report)
	}
}

func TestAnalyzeGoroutinesLabelsHTTPConnectionWithoutClaimingActiveRequest(t *testing.T) {
	report, err := AnalyzeGoroutines(`goroutine 20 [IO wait]:
internal/poll.runtime_pollWait(0x1, 0x72)
	/usr/local/go/src/runtime/netpoll.go:351 +0x85
net/http.(*conn).serve(0xc000100000, {0x1, 0x2})
	/usr/local/go/src/net/http/server.go:2127 +0x52d
`)
	if err != nil {
		t.Fatalf("analyze goroutines: %v", err)
	}
	finding := findingByCode(t, report.Groups[0].Findings, FindingHTTPConnectionNetHTTP)
	if strings.Contains(strings.ToLower(finding.Description), "in-flight") || strings.Contains(strings.ToLower(finding.Description), "request") {
		t.Fatalf("description %q incorrectly claims an active request", finding.Description)
	}
}

func TestAnalyzeGoroutinesUsesDeterministicOrderForTies(t *testing.T) {
	report, err := AnalyzeGoroutines(`goroutine 1 [running]:
main.zeta()
	/srv/main.go:2

goroutine 2 [running]:
main.alpha()
	/srv/main.go:1
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Groups) != 2 || report.Groups[0].Stack[0] != "main.alpha()" {
		t.Fatalf("groups = %#v, want stable lexical tie-break", report.Groups)
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

func findingByCode(t *testing.T, findings []Finding, code string) Finding {
	t.Helper()
	for _, finding := range findings {
		if finding.Code == code {
			return finding
		}
	}
	t.Fatalf("finding %q not found in %#v", code, findings)
	return Finding{}
}
