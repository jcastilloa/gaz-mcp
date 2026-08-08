//go:build !linux

package diagnostics

func collectProcessMetrics(string) map[string]any {
	return map[string]any{}
}
