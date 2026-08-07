package diagnostics

import (
	"encoding/json"
	"time"
)

func parseExpvars(data []byte) (map[string]any, error) {
	vars := make(map[string]any)
	if err := json.Unmarshal(data, &vars); err != nil {
		return nil, err
	}
	return vars, nil
}

func runtimeStatsFromExpvars(vars map[string]any) RuntimeStats {
	stats := RuntimeStats{Available: true, Instrumentation: "expvar"}
	stats.Goroutines = int(number(vars["num_goroutine"]))
	stats.Threads = int(number(vars["num_threads"]))
	memstats, ok := object(vars["memstats"])
	if !ok {
		return stats
	}
	stats.HeapAlloc = uint64(number(memstats["HeapAlloc"]))
	stats.HeapSys = uint64(number(memstats["HeapSys"]))
	stats.NumGC = uint32(number(memstats["NumGC"]))
	stats.PauseTotal = time.Duration(number(memstats["PauseTotalNs"]))
	if lastGC := int64(number(memstats["LastGC"])); lastGC > 0 {
		stats.LastGC = time.Unix(0, lastGC)
	}
	return stats
}

func dbPoolStatsFromExpvars(vars map[string]any) []DBPoolStats {
	diagnostics, ok := object(vars["gaz_diagnostics"])
	if !ok {
		return nil
	}
	pools, ok := object(diagnostics["db_pools"])
	if !ok {
		return nil
	}
	result := make([]DBPoolStats, 0, len(pools))
	for name, value := range pools {
		pool, ok := object(value)
		if !ok {
			continue
		}
		result = append(result, DBPoolStats{
			Name: name, OpenConnections: int(number(pool["open_connections"])), InUse: int(number(pool["in_use"])), Idle: int(number(pool["idle"])),
			MaxOpenConnections: int(number(pool["max_open_connections"])), WaitCount: int64(number(pool["wait_count"])), WaitDuration: time.Duration(number(pool["wait_duration_ns"])),
			MaxIdleClosed: int64(number(pool["max_idle_closed"])), MaxIdleTimeClosed: int64(number(pool["max_idle_time_closed"])), MaxLifetimeClosed: int64(number(pool["max_lifetime_closed"])),
		})
	}
	return result
}

func object(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	return object, ok
}

func number(value any) float64 {
	number, ok := value.(float64)
	if !ok {
		return 0
	}
	return number
}
