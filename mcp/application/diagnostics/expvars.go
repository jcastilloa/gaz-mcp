package diagnostics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"
)

func parseExpvars(data []byte) (map[string]any, error) {
	vars := make(map[string]any)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&vars); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("expvar response contains multiple JSON values")
		}
		return nil, err
	}
	return vars, nil
}

func runtimeStatsFromExpvars(vars map[string]any) RuntimeStats {
	stats := RuntimeStats{Available: true, Instrumentation: "expvar"}
	if value, ok := vars["num_goroutine"]; ok {
		stats.Goroutines = int(number(value))
		stats.GoroutineSource = "expvar:num_goroutine"
	}
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
	result := []DBPoolStats{}
	diagnostics, ok := object(vars["gaz_diagnostics"])
	if !ok {
		return result
	}
	pools, ok := object(diagnostics["db_pools"])
	if !ok {
		return result
	}
	result = make([]DBPoolStats, 0, len(pools))
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
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func object(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	return object, ok
}

func number(value any) int64 {
	switch value := value.(type) {
	case json.Number:
		number, err := value.Int64()
		if err == nil {
			return number
		}
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	}
	return 0
}
