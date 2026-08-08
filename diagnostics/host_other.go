//go:build !linux

package diagnostics

import "runtime"

func collectHostMetrics(string) map[string]any {
	return map[string]any{"logical_cpus": runtime.NumCPU()}
}
