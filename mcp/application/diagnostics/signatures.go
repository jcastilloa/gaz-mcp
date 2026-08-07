package diagnostics

import "strings"

type signature struct {
	code        string
	severity    string
	description string
	hint        string
	matches     func(GoroutineGroup) bool
}

var signatures = []signature{
	{
		code: FindingDBPoolExhausted, severity: "critical",
		description: "Goroutines are waiting for a database/sql connection.",
		hint:        "Check SetMaxOpenConns, unclosed rows, uncommitted transactions, and slow queries.",
		matches: func(group GoroutineGroup) bool {
			return containsStack(group, "database/sql.(*DB).conn") && isWaitingState(group.State)
		},
	},
	{
		code: FindingHTTPInflightFastHTTP, severity: "info",
		description: "In-flight fasthttp requests share this stack.",
		matches:     func(group GoroutineGroup) bool { return containsStack(group, "fasthttp.(*Server).serveConn") },
	},
	{
		code: FindingHTTPInflightNetHTTP, severity: "info",
		description: "In-flight net/http requests share this stack.",
		matches:     func(group GoroutineGroup) bool { return containsStack(group, "net/http.(*conn).serve") },
	},
	{
		code: FindingLockContention, severity: "warning",
		description: "Several goroutines are blocked acquiring a mutex.",
		hint:        "Inspect the contended critical section and mutex profile.",
		matches: func(group GoroutineGroup) bool {
			return group.Count > 1 && (containsStack(group, "sync.(*Mutex).Lock") || containsStack(group, "sync.runtime_SemacquireMutex"))
		},
	},
	{
		code: FindingPossibleGoroutineLeak, severity: "warning",
		description: "A large group has remained blocked on a channel.",
		hint:        "Verify producers, cancellation paths, and goroutine ownership.",
		matches: func(group GoroutineGroup) bool {
			return group.Count >= 10 && group.BlockedFor >= timeToSuspectLeak && (group.State == "chan receive" || group.State == "chan send")
		},
	},
}

const timeToSuspectLeak = 5 * 60 * 1_000_000_000

func matchSignatures(group GoroutineGroup) []Finding {
	findings := make([]Finding, 0, 2)
	for _, signature := range signatures {
		if signature.matches(group) {
			findings = append(findings, Finding{Code: signature.code, Severity: signature.severity, Description: signature.description, Hint: signature.hint})
		}
	}
	return findings
}

func containsStack(group GoroutineGroup, pattern string) bool {
	for _, line := range group.Stack {
		if strings.Contains(line, pattern) {
			return true
		}
	}
	return false
}

func isWaitingState(state string) bool {
	return state == "semacquire" || state == "chan receive" || state == "select"
}
