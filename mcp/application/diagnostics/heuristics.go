package diagnostics

import (
	"fmt"
	"sort"
	"time"
)

const (
	pressureRatio        = 0.80
	criticalPressure     = 0.95
	throttledPeriodRatio = 0.10
	throttledTimeRatio   = 0.01
	mutexWaitRatio       = 0.05
	minimumFDGrowth      = 32
	minimumRSSGrowth     = 32 << 20
)

func analyzeMetricSnapshots(before, after MetricSnapshot, goroutines GoroutineReport) []Suspicion {
	window := sampleDuration(before, after)
	var suspicions []Suspicion
	suspicions = append(suspicions, fdSuspicions(before, after)...)
	suspicions = append(suspicions, hostSuspicions(after)...)
	suspicions = append(suspicions, memorySuspicions(before, after)...)
	suspicions = append(suspicions, cpuSuspicions(before, after, window)...)
	suspicions = append(suspicions, schedulerSuspicions(before, after, window, goroutines)...)
	suspicions = append(suspicions, pidSuspicions(after)...)
	suspicions = append(suspicions, dbPoolSuspicions(before, after, goroutines)...)
	suspicions = append(suspicions, goroutineSuspicions(goroutines, suspicions)...)
	sort.SliceStable(suspicions, func(i, j int) bool {
		return severityRank(suspicions[i].Severity) > severityRank(suspicions[j].Severity)
	})
	if suspicions == nil {
		return []Suspicion{}
	}
	return suspicions
}

func hostSuspicions(after MetricSnapshot) []Suspicion {
	var result []Suspicion
	total, totalOK := numberAtOK(after.Host, "memory_total_bytes")
	available, availableOK := numberAtOK(after.Host, "memory_available_bytes")
	if totalOK && availableOK && total > 0 && available/total <= 0.10 {
		severity := "warning"
		if available/total <= 0.05 {
			severity = "critical"
		}
		result = append(result, Suspicion{
			Code: FindingHostMemoryPressure, Severity: severity, Confidence: ConfidenceSuspected,
			Reason: fmt.Sprintf("The host has %.1f%% of physical memory available.", available/total*100),
			Evidence: []MetricEvidence{
				evidence("host.memory_available_bytes", after.Host, "memory_available_bytes"),
				evidence("host.memory_total_bytes", after.Host, "memory_total_bytes"),
			},
			Verify: "Check host memory PSI and swap activity, then compare the process cgroup limit before attributing pressure to this process.",
		})
	}
	cpuPressure, cpuPressureOK := numberAtOK(after.Host, "cpu_pressure", "some_avg10")
	if cpuPressureOK && cpuPressure >= 20 {
		result = append(result, Suspicion{
			Code: FindingHostCPUPressure, Severity: "warning", Confidence: ConfidenceSuspected,
			Reason:   fmt.Sprintf("Host tasks were delayed for CPU during %.1f%% of the latest 10-second PSI window.", cpuPressure),
			Evidence: []MetricEvidence{evidence("host.cpu_pressure.some_avg10", after.Host, "cpu_pressure", "some_avg10")},
			Verify:   "Compare host load with logical CPUs and this process cgroup throttling before attributing the contention.",
		})
	}
	allocated, allocatedOK := numberAtOK(after.Host, "file_handles_allocated")
	limit, limitOK := numberAtOK(after.Host, "file_handles_max")
	if allocatedOK && limitOK && limit > 0 && allocated/limit >= pressureRatio {
		severity := "warning"
		if allocated/limit >= criticalPressure {
			severity = "critical"
		}
		result = append(result, Suspicion{
			Code: FindingSystemFileTablePressure, Severity: severity, Confidence: ConfidenceSuspected,
			Reason: fmt.Sprintf("The host uses %.1f%% of its system-wide file handle table.", allocated/limit*100),
			Evidence: []MetricEvidence{
				evidence("host.file_handles_allocated", after.Host, "file_handles_allocated"),
				evidence("host.file_handles_max", after.Host, "file_handles_max"),
			},
			Verify: "Compare process open-file counts across the host before changing the system-wide limit.",
		})
	}
	return result
}

func clearGoroutineFindings(report *DiagnosisReport) {
	for groupIndex := range report.Goroutines.Groups {
		report.Goroutines.Groups[groupIndex].Findings = nil
	}
}

func goroutineSuspicions(goroutines GoroutineReport, existing []Suspicion) []Suspicion {
	var result []Suspicion
	for _, group := range goroutines.Groups {
		for _, finding := range group.Findings {
			if finding.Code == FindingDBPoolExhausted || finding.Code == FindingHTTPConnectionFastHTTP || finding.Code == FindingHTTPConnectionNetHTTP {
				continue
			}
			if finding.Code == FindingLockContention && hasSuspicion(existing, FindingMutexContention) {
				continue
			}
			result = append(result, Suspicion{
				Code: finding.Code, Severity: finding.Severity, Confidence: ConfidenceSuspected,
				Reason: finding.Description,
				Evidence: []MetricEvidence{
					{Metric: "goroutines.group.count", Value: group.Count},
					{Metric: "goroutines.group.state", Value: group.State},
					{Metric: "goroutines.group.blocked_for_ns", Value: group.BlockedFor.Nanoseconds()},
					{Metric: "goroutines.group.stack", Value: group.Stack},
				},
				Verify: finding.Hint,
			})
		}
	}
	return result
}

func hasSuspicion(suspicions []Suspicion, code string) bool {
	for _, suspicion := range suspicions {
		if suspicion.Code == code {
			return true
		}
	}
	return false
}

func fdSuspicions(before, after MetricSnapshot) []Suspicion {
	open, openOK := numberAtOK(after.Process, "open_fds")
	limit, limitOK := numberAtOK(after.Process, "fd_soft_limit")
	if !openOK || !limitOK || limit <= 0 {
		return nil
	}
	var result []Suspicion
	ratio := open / limit
	if ratio >= pressureRatio {
		severity := "warning"
		if ratio >= criticalPressure {
			severity = "critical"
		}
		result = append(result, Suspicion{
			Code: FindingFDLimitPressure, Severity: severity, Confidence: ConfidenceSuspected,
			Reason: fmt.Sprintf("The process uses %.1f%% of its soft file descriptor limit.", ratio*100),
			Evidence: []MetricEvidence{
				evidence("process.open_fds", after.Process, "open_fds"),
				evidence("process.fd_soft_limit", after.Process, "fd_soft_limit"),
			},
			Verify: "Take another sample and inspect whether file descriptors continue growing under stable load.",
		})
	}
	previousOpen, hasBaseline := numberAtOK(before.Process, "open_fds")
	growth := open - previousOpen
	if hasBaseline && growth >= minimumFDGrowth && open > 0 && growth/open >= 0.05 {
		result = append(result, Suspicion{
			Code: FindingPossibleFDLeak, Severity: "warning", Confidence: ConfidenceSuspected,
			Reason:   fmt.Sprintf("Open file descriptors increased by %.0f during the sample window.", growth),
			Evidence: []MetricEvidence{{Metric: "process.open_fds", Value: open, Previous: numberAt(before.Process, "open_fds")}},
			Verify:   "Repeat across at least three windows and correlate growth with socket, file and request activity.",
		})
	}
	return result
}

func memorySuspicions(before, after MetricSnapshot) []Suspicion {
	var result []Suspicion
	current, currentOK := numberAtOK(after.Cgroup, "memory_current_bytes")
	limit, limitOK := numberAtOK(after.Cgroup, "memory_max_bytes")
	oomDelta := eventDelta(before, after, "oom")
	oomKillDelta := eventDelta(before, after, "oom_kill")
	underPressure := currentOK && limitOK && limit > 0 && current/limit >= 0.85
	if underPressure || oomDelta > 0 || oomKillDelta > 0 {
		ratio := current / limit
		severity := "warning"
		if ratio >= criticalPressure || oomDelta > 0 || oomKillDelta > 0 {
			severity = "critical"
		}
		reason := fmt.Sprintf("The cgroup uses %.1f%% of its memory limit.", ratio*100)
		if oomDelta > 0 || oomKillDelta > 0 {
			reason = "The cgroup reported new out-of-memory events during the sample window."
		}
		var evidenceItems []MetricEvidence
		if currentOK {
			evidenceItems = append(evidenceItems, evidence("cgroup.memory_current_bytes", after.Cgroup, "memory_current_bytes"))
		}
		if limitOK {
			evidenceItems = append(evidenceItems, evidence("cgroup.memory_max_bytes", after.Cgroup, "memory_max_bytes"))
		}
		if oomDelta > 0 {
			evidenceItems = append(evidenceItems, MetricEvidence{Metric: "cgroup.memory_events.oom.delta", Value: oomDelta})
		}
		if oomKillDelta > 0 {
			evidenceItems = append(evidenceItems, MetricEvidence{Metric: "cgroup.memory_events.oom_kill.delta", Value: oomKillDelta})
		}
		result = append(result, Suspicion{
			Code: FindingCgroupMemoryPressure, Severity: severity, Confidence: ConfidenceSuspected,
			Reason: reason, Evidence: evidenceItems,
			Verify: "Compare RSS, Go runtime memory and a heap profile to identify whether pressure comes from Go heap or native memory.",
		})
	}
	goLimit, goLimitOK := numberAtOK(after.Runtime, "memory_limit_bytes")
	if limitOK && goLimitOK && limit > 0 && goLimit > limit*0.95 {
		result = append(result, Suspicion{
			Code: FindingMemoryLimitMismatch, Severity: "warning", Confidence: ConfidenceSuspected,
			Reason: "The Go memory limit leaves little or no cgroup headroom for stacks, mappings and native allocations.",
			Evidence: []MetricEvidence{
				evidence("runtime.memory_limit_bytes", after.Runtime, "memory_limit_bytes"),
				evidence("cgroup.memory_max_bytes", after.Cgroup, "memory_max_bytes"),
			},
			Verify: "Account for non-heap memory and set GOMEMLIMIT below the cgroup limit with operational headroom.",
		})
	}
	rssGrowth := deltaAt(before.Process, after.Process, "rss_bytes")
	runtimeGrowth := deltaAt(before.Runtime, after.Runtime, "runtime_memory_bytes")
	_, hasRSSBaseline := valueAt(before.Process, "rss_bytes")
	_, hasRuntimeBaseline := valueAt(before.Runtime, "runtime_memory_bytes")
	if hasRSSBaseline && hasRuntimeBaseline && rssGrowth >= minimumRSSGrowth && runtimeGrowth < rssGrowth/2 {
		result = append(result, Suspicion{
			Code: FindingNativeMemoryPressure, Severity: "warning", Confidence: ConfidenceSuspected,
			Reason: "RSS grew substantially without comparable growth in Go runtime memory.",
			Evidence: []MetricEvidence{
				{Metric: "process.rss_bytes", Value: numberAt(after.Process, "rss_bytes"), Previous: numberAt(before.Process, "rss_bytes")},
				{Metric: "runtime.runtime_memory_bytes", Value: numberAt(after.Runtime, "runtime_memory_bytes"), Previous: numberAt(before.Runtime, "runtime_memory_bytes")},
			},
			Verify: "Inspect cgo, mmap and native libraries; a Go heap profile alone may not explain this growth.",
		})
	}
	return result
}

func cpuSuspicions(before, after MetricSnapshot, window time.Duration) []Suspicion {
	var result []Suspicion
	periods := deltaAt(before.Cgroup, after.Cgroup, "cpu_periods")
	throttled := deltaAt(before.Cgroup, after.Cgroup, "cpu_throttled_periods")
	throttledUsec := deltaAt(before.Cgroup, after.Cgroup, "cpu_throttled_usec")
	windowUsec := window.Seconds() * 1_000_000
	if periods > 0 && throttled > 0 && windowUsec > 0 &&
		throttled/periods >= throttledPeriodRatio && throttledUsec/windowUsec >= throttledTimeRatio {
		result = append(result, Suspicion{
			Code: FindingCPUThrottled, Severity: "warning", Confidence: ConfidenceSuspected,
			Reason: fmt.Sprintf("%.1f%% of cgroup CPU periods were throttled during the sample window.", throttled/periods*100),
			Evidence: []MetricEvidence{
				{Metric: "cgroup.cpu_periods.delta", Value: periods},
				{Metric: "cgroup.cpu_throttled_periods.delta", Value: throttled},
				{Metric: "cgroup.cpu_throttled_usec.delta", Value: throttledUsec},
			},
			Verify: "Capture a CPU profile and compare runnable goroutines with the configured CPU quota.",
		})
	}
	seconds := window.Seconds()
	gcCPU := deltaAt(before.Runtime, after.Runtime, "gc_cpu_seconds")
	userCPU := deltaAt(before.Runtime, after.Runtime, "user_cpu_seconds")
	allocations := deltaAt(before.Runtime, after.Runtime, "heap_allocated_bytes_total")
	if seconds > 0 && gcCPU > 0 && gcCPU/(gcCPU+userCPU) >= 0.25 && allocations/seconds > 0 {
		result = append(result, Suspicion{
			Code: FindingGCPressure, Severity: "warning", Confidence: ConfidenceSuspected,
			Reason: "Garbage collection consumed a high share of measured Go CPU while allocations continued.",
			Evidence: []MetricEvidence{
				{Metric: "runtime.gc_cpu_seconds.delta", Value: gcCPU},
				{Metric: "runtime.user_cpu_seconds.delta", Value: userCPU},
				{Metric: "runtime.heap_allocated_bytes_total.delta", Value: allocations},
			},
			Verify: "Capture an allocs profile and inspect the dominant allocation sites.",
		})
	}
	return result
}

func schedulerSuspicions(before, after MetricSnapshot, window time.Duration, goroutines GoroutineReport) []Suspicion {
	var result []Suspicion
	previousRunnable, previousRunnableOK := numberAtOK(before.Runtime, "goroutines", "runnable")
	previousGOMAXPROCS, previousGOMAXPROCSOK := numberAtOK(before.Runtime, "gomaxprocs")
	runnable, runnableOK := numberAtOK(after.Runtime, "goroutines", "runnable")
	gomaxprocs, gomaxprocsOK := numberAtOK(after.Runtime, "gomaxprocs")
	if previousRunnableOK && previousGOMAXPROCSOK && runnableOK && gomaxprocsOK &&
		previousGOMAXPROCS > 0 && gomaxprocs > 0 &&
		previousRunnable > previousGOMAXPROCS && runnable > gomaxprocs {
		result = append(result, Suspicion{
			Code: FindingSchedulerPressure, Severity: "warning", Confidence: ConfidenceSuspected,
			Reason: fmt.Sprintf("There are %.0f runnable goroutines for %.0f available Go processors.", runnable, gomaxprocs),
			Evidence: []MetricEvidence{
				evidence("runtime.goroutines.runnable", after.Runtime, "goroutines", "runnable"),
				evidence("runtime.gomaxprocs", after.Runtime, "gomaxprocs"),
			},
			Verify: "Check scheduler latency and capture a CPU profile or execution trace during the same load.",
		})
	}
	seconds := window.Seconds()
	waitDelta := deltaAt(before.Runtime, after.Runtime, "mutex_wait_seconds")
	if seconds > 0 && waitDelta/seconds >= mutexWaitRatio {
		confidence := ConfidenceSuspected
		if goroutineFindingCount(goroutines, FindingLockContention) > 0 {
			confidence = ConfidenceConfirmed
		}
		result = append(result, Suspicion{
			Code: FindingMutexContention, Severity: "warning", Confidence: confidence,
			Reason:   "Cumulative mutex wait time increased materially during the sample window.",
			Evidence: []MetricEvidence{{Metric: "runtime.mutex_wait_seconds.delta", Value: waitDelta}},
			Verify:   "Capture a mutex profile and inspect the contended critical sections.",
		})
	}
	return result
}

func pidSuspicions(after MetricSnapshot) []Suspicion {
	current, currentOK := numberAtOK(after.Cgroup, "pids_current")
	limit, limitOK := numberAtOK(after.Cgroup, "pids_max")
	if !currentOK || !limitOK || limit <= 0 || current/limit < pressureRatio {
		return nil
	}
	return []Suspicion{{
		Code: FindingPIDPressure, Severity: "warning", Confidence: ConfidenceSuspected,
		Reason: fmt.Sprintf("The cgroup uses %.1f%% of its PID limit.", current/limit*100),
		Evidence: []MetricEvidence{
			evidence("cgroup.pids_current", after.Cgroup, "pids_current"),
			evidence("cgroup.pids_max", after.Cgroup, "pids_max"),
		},
		Verify: "Inspect OS thread growth, cgo activity and the threadcreate profile.",
	}}
}

func dbPoolSuspicions(before, after MetricSnapshot, goroutines GoroutineReport) []Suspicion {
	waiters := goroutineFindingCount(goroutines, FindingDBPoolExhausted)
	beforePools := poolsByName(before.DBPools)
	var result []Suspicion
	correlated := false
	for _, pool := range after.DBPools {
		previous, hasBaseline := beforePools[pool.Name]
		waitDelta := int64(0)
		if hasBaseline {
			waitDelta = pool.WaitCount - previous.WaitCount
			if waitDelta < 0 {
				waitDelta = 0
			}
		}
		saturated := pool.MaxOpenConnections > 0 && pool.InUse >= pool.MaxOpenConnections && pool.Idle == 0
		if waiters == 0 && waitDelta == 0 {
			continue
		}
		if !saturated && waitDelta == 0 {
			continue
		}
		confidence := ConfidenceSuspected
		if waiters > 0 && saturated && pool.WaitCount > 0 && len(after.DBPools) == 1 {
			confidence = ConfidenceConfirmed
		}
		result = append(result, Suspicion{
			Code: FindingDBPoolExhausted, Severity: "critical", Confidence: confidence,
			Reason: fmt.Sprintf("Database pool %q shows current connection pressure.", pool.Name),
			Evidence: []MetricEvidence{
				{Metric: "goroutines.database_sql_waiters", Value: waiters},
				{Metric: "db_pools." + pool.Name + ".in_use", Value: pool.InUse},
				{Metric: "db_pools." + pool.Name + ".max_open_connections", Value: pool.MaxOpenConnections},
				{Metric: "db_pools." + pool.Name + ".wait_count.delta", Value: waitDelta},
			},
			Verify: "Match blocked request stacks to this pool and inspect slow queries, unclosed rows and transaction completion.",
		})
		correlated = true
	}
	if waiters > 0 && !correlated {
		reason := "Goroutines are waiting in database/sql but pool metrics are unavailable."
		verify := "Publish DBStats for the relevant pools to validate saturation and current wait growth."
		if len(after.DBPools) > 0 {
			reason = "Goroutines are waiting in database/sql, but the published pool metrics do not identify current saturation or wait growth."
			verify = "Verify that every relevant *sql.DB is instrumented and match the waiter stacks to the correct pool before attributing exhaustion."
		}
		result = append(result, Suspicion{
			Code: FindingDBPoolExhausted, Severity: "warning", Confidence: ConfidenceSuspected,
			Reason:   reason,
			Evidence: []MetricEvidence{{Metric: "goroutines.database_sql_waiters", Value: waiters}},
			Verify:   verify,
		})
	}
	return result
}

func evidence(name string, values map[string]any, path ...string) MetricEvidence {
	value, _ := valueAt(values, path...)
	return MetricEvidence{Metric: name, Value: value}
}

func eventDelta(before, after MetricSnapshot, event string) float64 {
	return deltaAt(before.Cgroup, after.Cgroup, "memory_events", event)
}

func goroutineFindingCount(report GoroutineReport, code string) int {
	count := 0
	for _, group := range report.Groups {
		if hasFindingCode(group.Findings, code) {
			count += group.Count
		}
	}
	return count
}

func poolsByName(pools []DBPoolStats) map[string]DBPoolStats {
	result := make(map[string]DBPoolStats, len(pools))
	for _, pool := range pools {
		result[pool.Name] = pool
	}
	return result
}

func severityRank(severity string) int {
	switch severity {
	case "critical":
		return 2
	case "warning":
		return 1
	default:
		return 0
	}
}
