//go:build !linux

package diagnostics

func collectCgroupMetrics(string, string) map[string]any {
	return map[string]any{}
}
