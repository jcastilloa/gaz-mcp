package diagnostics

import (
	"fmt"
	"strings"
)

func BuildDiagnosis(runtime RuntimeStats, goroutines GoroutineReport, pools []DBPoolStats) DiagnosisReport {
	report := DiagnosisReport{Runtime: runtime, Goroutines: goroutines, DBPools: pools}
	if hasDBPoolWaiters(goroutines) {
		report.Conclusions = append(report.Conclusions, poolConclusion(goroutines, pools))
	}
	return report
}

func hasDBPoolWaiters(report GoroutineReport) bool {
	for _, group := range report.Groups {
		for _, finding := range group.Findings {
			if finding.Code == FindingDBPoolExhausted {
				return true
			}
		}
	}
	return false
}

func poolConclusion(goroutines GoroutineReport, pools []DBPoolStats) Conclusion {
	waiting := dbPoolWaiterCount(goroutines)
	if pool, ok := exhaustedPool(pools); ok {
		return Conclusion{
			Code: FindingDBPoolExhausted, Severity: "critical", Confidence: ConfidenceConfirmed,
			Summary:  "Database connection pool exhausted.",
			Evidence: fmt.Sprintf("%d goroutines wait in database/sql; pool %q is in use %d/%d with %d idle connections and %d waits over %s.", waiting, pool.Name, pool.InUse, pool.MaxOpenConnections, pool.Idle, pool.WaitCount, pool.WaitDuration),
			Hint:     "Review SetMaxOpenConns, rows.Close, transaction completion, and slow queries.",
		}
	}
	return Conclusion{
		Code: FindingDBPoolExhausted, Severity: "warning", Confidence: ConfidenceSuspected,
		Summary:  "Possible database connection pool exhaustion.",
		Evidence: fmt.Sprintf("%d goroutines wait in database/sql, but DB pool metrics are unavailable or do not show saturation.", waiting),
		Hint:     "Expose database/sql DBStats in /debug/vars to confirm saturation.",
	}
}

func dbPoolWaiterCount(report GoroutineReport) int {
	count := 0
	for _, group := range report.Groups {
		if hasFindingCode(group.Findings, FindingDBPoolExhausted) {
			count += group.Count
		}
	}
	return count
}

func exhaustedPool(pools []DBPoolStats) (DBPoolStats, bool) {
	for _, pool := range pools {
		if pool.MaxOpenConnections > 0 && pool.InUse >= pool.MaxOpenConnections && pool.Idle == 0 && pool.WaitCount > 0 {
			return pool, true
		}
	}
	return DBPoolStats{}, false
}

func hasFindingCode(findings []Finding, code string) bool {
	for _, finding := range findings {
		if strings.EqualFold(finding.Code, code) {
			return true
		}
	}
	return false
}
