package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	diagnosticsApp "github.com/jcastillo/gaz-mcp/mcp/application/diagnostics"
	"github.com/mark3labs/mcp-go/mcp"
)

type Diagnostics struct {
	service diagnosticsApp.Service
}

type dbPoolStatsResult struct {
	Pools []diagnosticsApp.DBPoolStats `json:"pools"`
}

func NewDiagnostics(service diagnosticsApp.Service) Diagnostics {
	return Diagnostics{service: service}
}

func (d Diagnostics) CaptureProfileDefinition() mcp.Tool {
	return mcp.NewTool("capture_profile",
		mcp.WithDescription("Capture and persist a Go pprof profile from target (HTTP(S) base URL, e.g. http://service:6060). CPU identifies hot paths; heap identifies retained memory; allocs identifies allocation churn; goroutine identifies leaks and blocking; block and mutex identify contention; threadcreate identifies excessive OS threads."),
		targetParameter(),
		mcp.WithString("type", mcp.Required(), mcp.Enum("cpu", "heap", "allocs", "goroutine", "block", "mutex", "threadcreate"), mcp.Description("Profile type to capture.")),
		mcp.WithNumber("seconds", mcp.Description("CPU capture duration in seconds; 1–60, default 10. Ignored for non-CPU profiles."), mcp.Min(1), mcp.Max(diagnosticsApp.MaxCPUProfileSeconds)),
		captureProfileTool(),
	)
}

func (d Diagnostics) CaptureProfileHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target, result := requiredTarget(request)
	if result != nil {
		return result, nil
	}
	profileType := strings.TrimSpace(mcp.ParseString(request, "type", ""))
	metadata, err := d.service.CaptureProfile(ctx, target, profileType, mcp.ParseInt(request, "seconds", 0))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("capture profile: %v", err)), nil
	}
	return jsonToolResult(metadata)
}

func (d Diagnostics) AnalyzeProfileDefinition() mcp.Tool {
	return mcp.NewTool("analyze_profile",
		mcp.WithDescription("Analyze a previously captured pprof profile by ID. Returns top functions ordered by cumulative cost, with flat and cumulative values."),
		mcp.WithString("profile_id", mcp.Required(), mcp.Description("ID returned by capture_profile.")),
		mcp.WithNumber("top_n", mcp.Description("Maximum functions to return; default 20."), mcp.Min(1), mcp.Max(100)),
		readOnlyTool(),
	)
}

func (d Diagnostics) AnalyzeProfileHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, result := requiredString(request, "profile_id")
	if result != nil {
		return result, nil
	}
	analysis, err := d.service.AnalyzeProfile(ctx, id, topN(request))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("analyze profile: %v", err)), nil
	}
	return jsonToolResult(analysis)
}

func (d Diagnostics) DiffProfilesDefinition() mcp.Tool {
	return mcp.NewTool("diff_profiles",
		mcp.WithDescription("Compare two captured pprof profiles of the same primary sample type. Positive values in the result grew from profile A to profile B."),
		mcp.WithString("id_a", mcp.Required(), mcp.Description("Earlier or baseline profile ID.")),
		mcp.WithString("id_b", mcp.Required(), mcp.Description("Later or comparison profile ID.")),
		mcp.WithNumber("top_n", mcp.Description("Maximum functions to return; default 20."), mcp.Min(1), mcp.Max(100)),
		readOnlyTool(),
	)
}

func (d Diagnostics) DiffProfilesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	first, result := requiredString(request, "id_a")
	if result != nil {
		return result, nil
	}
	second, result := requiredString(request, "id_b")
	if result != nil {
		return result, nil
	}
	diff, err := d.service.DiffProfiles(ctx, first, second, topN(request))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("diff profiles: %v", err)), nil
	}
	return jsonToolResult(diff)
}

func (d Diagnostics) GenerateFlamegraphDefinition() mcp.Tool {
	return mcp.NewTool("generate_flamegraph",
		mcp.WithDescription("Generate an SVG flame graph for a captured pprof profile."),
		mcp.WithString("profile_id", mcp.Required(), mcp.Description("ID returned by capture_profile.")),
		readOnlyTool(),
	)
}

func (d Diagnostics) GenerateFlamegraphHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, result := requiredString(request, "profile_id")
	if result != nil {
		return result, nil
	}
	image, err := d.service.GenerateFlamegraph(ctx, id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("generate flamegraph: %v", err)), nil
	}
	return mcp.NewToolResultImage("Flame graph generated from captured profile.", base64.StdEncoding.EncodeToString(image), "image/svg+xml"), nil
}

func (d Diagnostics) AnalyzeGoroutinesDefinition() mcp.Tool {
	return mcp.NewTool("analyze_goroutines",
		mcp.WithDescription("Fetch target/debug/pprof/goroutine?debug=2, group equivalent stacks, preserve blocked duration, and label known patterns such as database/sql pool waits, HTTP requests, lock contention, and possible goroutine leaks."),
		targetParameter(),
		readOnlyTool(),
	)
}

func (d Diagnostics) AnalyzeGoroutinesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target, result := requiredTarget(request)
	if result != nil {
		return result, nil
	}
	report, err := d.service.AnalyzeGoroutines(ctx, target)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("analyze goroutines: %v", err)), nil
	}
	return jsonToolResult(report)
}

func (d Diagnostics) GetRuntimeStatsDefinition() mcp.Tool {
	return mcp.NewTool("get_runtime_stats",
		mcp.WithDescription("Read runtime stats from target/debug/vars: goroutine count plus expvar memstats and GC summary. Works with the standard expvar endpoint."),
		targetParameter(),
		readOnlyTool(),
	)
}

func (d Diagnostics) GetRuntimeStatsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target, result := requiredTarget(request)
	if result != nil {
		return result, nil
	}
	stats, err := d.service.GetRuntimeStats(ctx, target)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get runtime stats: %v", err)), nil
	}
	return jsonToolResult(stats)
}

func (d Diagnostics) GetDBPoolStatsDefinition() mcp.Tool {
	return mcp.NewTool("get_db_pool_stats",
		mcp.WithDescription("Read optional database/sql pool stats from target/debug/vars under gaz_diagnostics.db_pools. Returns an object with a pools array, which is empty when the target does not use SQL or has not installed the optional instrumentation."),
		targetParameter(),
		readOnlyTool(),
	)
}

func (d Diagnostics) GetDBPoolStatsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target, result := requiredTarget(request)
	if result != nil {
		return result, nil
	}
	pools, err := d.service.GetDBPoolStats(ctx, target)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get DB pool stats: %v", err)), nil
	}
	if pools == nil {
		pools = []diagnosticsApp.DBPoolStats{}
	}
	return jsonToolResult(dbPoolStatsResult{Pools: pools})
}

func (d Diagnostics) GetExpvarsDefinition() mcp.Tool {
	return mcp.NewTool("get_expvars",
		mcp.WithDescription("Return the complete JSON object published by target/debug/vars, including standard expvar values and service-specific variables."),
		targetParameter(),
		readOnlyTool(),
	)
}

func (d Diagnostics) GetExpvarsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target, result := requiredTarget(request)
	if result != nil {
		return result, nil
	}
	vars, err := d.service.GetExpvars(ctx, target)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get expvars: %v", err)), nil
	}
	return jsonToolResult(vars)
}

func (d Diagnostics) DiagnoseDefinition() mcp.Tool {
	return mcp.NewTool("diagnose",
		mcp.WithDescription("Run the Go-process diagnostic sweep: goroutine stacks, expvar runtime stats, and optional database/sql pools. Correlates evidence and directly confirms DB_POOL_EXHAUSTED only when blocked database/sql waiters agree with saturated pool metrics."),
		targetParameter(),
		readOnlyTool(),
	)
}

func (d Diagnostics) DiagnoseHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target, result := requiredTarget(request)
	if result != nil {
		return result, nil
	}
	report, err := d.service.Diagnose(ctx, target)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("diagnose: %v", err)), nil
	}
	return jsonToolResult(report)
}

func targetParameter() mcp.ToolOption {
	return mcp.WithString("target", mcp.Required(), mcp.Description("Base HTTP(S) URL of the Go process exposing /debug/pprof and optionally /debug/vars; for example http://service:6060."))
}

func readOnlyTool() mcp.ToolOption {
	return mcp.WithToolAnnotation(mcp.ToolAnnotation{
		ReadOnlyHint:    mcp.ToBoolPtr(true),
		DestructiveHint: mcp.ToBoolPtr(false),
		IdempotentHint:  mcp.ToBoolPtr(true),
		OpenWorldHint:   mcp.ToBoolPtr(true),
	})
}

func captureProfileTool() mcp.ToolOption {
	return mcp.WithToolAnnotation(mcp.ToolAnnotation{
		ReadOnlyHint:    mcp.ToBoolPtr(false),
		DestructiveHint: mcp.ToBoolPtr(false),
		IdempotentHint:  mcp.ToBoolPtr(false),
		OpenWorldHint:   mcp.ToBoolPtr(true),
	})
}

func requiredTarget(request mcp.CallToolRequest) (string, *mcp.CallToolResult) {
	return requiredString(request, "target")
}

func requiredString(request mcp.CallToolRequest, name string) (string, *mcp.CallToolResult) {
	value := strings.TrimSpace(mcp.ParseString(request, name, ""))
	if value == "" {
		return "", mcp.NewToolResultError(name + " parameter is required")
	}
	return value, nil
}

func topN(request mcp.CallToolRequest) int {
	topN := mcp.ParseInt(request, "top_n", 20)
	if topN < 1 {
		return 20
	}
	return topN
}

func jsonToolResult(value any) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultJSON(value)
}
