package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	diagnosticsApp "github.com/jcastillo/gaz-mcp/mcp/application/diagnostics"
	diagnosticsDomain "github.com/jcastillo/gaz-mcp/mcp/domain/diagnostics"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestCaptureProfileIsNotIdempotentOrReadOnly(t *testing.T) {
	definition := Diagnostics{}.CaptureProfileDefinition()
	if definition.Annotations.ReadOnlyHint == nil || *definition.Annotations.ReadOnlyHint {
		t.Fatalf("readOnlyHint = %#v, want false", definition.Annotations.ReadOnlyHint)
	}
	if definition.Annotations.IdempotentHint == nil || *definition.Annotations.IdempotentHint {
		t.Fatalf("idempotentHint = %#v, want false", definition.Annotations.IdempotentHint)
	}
}

func TestGetDBPoolStatsReturnsObjectStructuredContent(t *testing.T) {
	service := diagnosticsApp.NewService(toolTargetRepository{expvars: []byte(`{
  "gaz_diagnostics": {"db_pools": {"primary": {
    "open_connections": 10, "in_use": 8, "idle": 2,
    "max_open_connections": 10, "wait_count": 7, "wait_duration_ns": 42
  }}}
}`)}, toolProfileStore{})
	result, err := NewDiagnostics(service).GetDBPoolStatsHandler(context.Background(), toolRequest("http://service:6060"))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	content, ok := result.StructuredContent.(dbPoolStatsResult)
	if !ok {
		t.Fatalf("structured content type = %T, want dbPoolStatsResult", result.StructuredContent)
	}
	if len(content.Pools) != 1 || content.Pools[0].Name != "primary" {
		t.Fatalf("pools = %#v, want primary pool", content.Pools)
	}
	assertStructuredContentIsObject(t, result, 1)
}

func TestGetDBPoolStatsReturnsEmptyPoolsArray(t *testing.T) {
	service := diagnosticsApp.NewService(toolTargetRepository{expvars: []byte(`{}`)}, toolProfileStore{})
	result, err := NewDiagnostics(service).GetDBPoolStatsHandler(context.Background(), toolRequest("http://service:6060"))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	content, ok := result.StructuredContent.(dbPoolStatsResult)
	if !ok {
		t.Fatalf("structured content type = %T, want dbPoolStatsResult", result.StructuredContent)
	}
	if content.Pools == nil || len(content.Pools) != 0 {
		t.Fatalf("pools = %#v, want empty non-nil array", content.Pools)
	}
	assertStructuredContentIsObject(t, result, 0)
}

func TestGetExpvarsIncludesInformationalInspectionAlert(t *testing.T) {
	alerts := []diagnosticsDomain.InspectionAlert{{Code: "TLS_SELF_SIGNED_CERTIFICATE", Severity: "info"}}
	service := diagnosticsApp.NewService(toolTargetRepository{expvars: []byte(`{"num_goroutine":1}`), alerts: alerts}, toolProfileStore{})

	result, err := NewDiagnostics(service).GetExpvarsHandler(context.Background(), toolRequest("https://service:6060"))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	content, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content type = %T, want map[string]any", result.StructuredContent)
	}
	value, ok := content["num_goroutine"].(json.Number)
	if !ok || value.String() != "1" {
		t.Fatalf("expvars = %#v, want num_goroutine", content)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content length = %d, want expvars plus one informational alert", len(result.Content))
	}
	alertText, ok := result.Content[1].(mcp.TextContent)
	if !ok || !strings.Contains(alertText.Text, `"severity":"info"`) {
		t.Fatalf("alert content = %#v, want informational alert", result.Content[1])
	}
}

func assertStructuredContentIsObject(t *testing.T, result *mcp.CallToolResult, poolCount int) {
	t.Helper()
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal tool result: %v", err)
	}
	var envelope struct {
		StructuredContent json.RawMessage `json:"structuredContent"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("unmarshal tool result: %v", err)
	}
	var content map[string]json.RawMessage
	if err := json.Unmarshal(envelope.StructuredContent, &content); err != nil {
		t.Fatalf("structured content JSON = %s, want object: %v", envelope.StructuredContent, err)
	}
	pools, ok := content["pools"]
	if !ok {
		t.Fatalf("structured content = %s, want pools property", envelope.StructuredContent)
	}
	var values []json.RawMessage
	if err := json.Unmarshal(pools, &values); err != nil {
		t.Fatalf("pools = %s, want array: %v", pools, err)
	}
	if len(values) != poolCount {
		t.Fatalf("pools length = %d, want %d", len(values), poolCount)
	}
}

func toolRequest(target string) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"target": target}}}
}

type toolTargetRepository struct {
	expvars []byte
	alerts  []diagnosticsDomain.InspectionAlert
}

func (repository toolTargetRepository) FetchProfile(context.Context, string, string, int) (diagnosticsDomain.FetchResult, error) {
	return diagnosticsDomain.FetchResult{}, nil
}

func (repository toolTargetRepository) FetchGoroutines(context.Context, string) (diagnosticsDomain.FetchResult, error) {
	return diagnosticsDomain.FetchResult{}, nil
}

func (repository toolTargetRepository) FetchExpvars(context.Context, string) (diagnosticsDomain.FetchResult, error) {
	return diagnosticsDomain.FetchResult{Data: repository.expvars, Alerts: repository.alerts}, nil
}

type toolProfileStore struct{}

func (toolProfileStore) Save(context.Context, diagnosticsDomain.CapturedProfile) (diagnosticsDomain.ProfileMetadata, error) {
	return diagnosticsDomain.ProfileMetadata{}, nil
}

func (toolProfileStore) Load(context.Context, string) (diagnosticsDomain.CapturedProfile, error) {
	return diagnosticsDomain.CapturedProfile{}, nil
}
