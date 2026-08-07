package diagnostics

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	profilepkg "github.com/google/pprof/profile"
	domain "github.com/jcastillo/gaz-mcp/mcp/domain/diagnostics"
)

func TestAnalyzeGoroutinesAcceptsRuntimeMetadataInHeader(t *testing.T) {
	dump := `goroutine 1 gp=0xc000002380 m=nil [running]:
main.main()
	/srv/main.go:10 +0x20
`

	report, err := AnalyzeGoroutines(dump)
	if err != nil {
		t.Fatalf("analyze goroutines: %v", err)
	}
	if report.Total != 1 {
		t.Fatalf("total = %d, want 1", report.Total)
	}
	if report.Groups[0].State != "running" {
		t.Fatalf("state = %q, want running", report.Groups[0].State)
	}
}

func TestAnalyzeGoroutinesAcceptsWindowsNewlines(t *testing.T) {
	report, err := AnalyzeGoroutines(strings.ReplaceAll(blockedRequests, "\n", "\r\n"))
	if err != nil {
		t.Fatalf("analyze goroutines: %v", err)
	}
	if report.Total != 3 || len(report.Groups) != 2 {
		t.Fatalf("report = %#v, want three goroutines in two groups", report)
	}
}

func TestAnalyzeProfileUsesDefaultSampleType(t *testing.T) {
	service := NewService(fakeTargetRepository{}, reviewProfileStore{profiles: map[string]domain.CapturedProfile{
		"heap": capturedTestProfile(t, "heap", 3, 1024),
	}})

	analysis, err := service.AnalyzeProfile(context.Background(), "heap", 20)
	if err != nil {
		t.Fatalf("analyze profile: %v", err)
	}
	if analysis.SampleType != "inuse_space" || analysis.Unit != "bytes" {
		t.Fatalf("sample = %s/%s, want inuse_space/bytes", analysis.SampleType, analysis.Unit)
	}
	if analysis.Total != 1024 {
		t.Fatalf("total = %d, want 1024", analysis.Total)
	}
	if len(analysis.Functions) != 1 || analysis.Functions[0].Flat != 1024 || analysis.Functions[0].Cumulative != 1024 {
		t.Fatalf("functions = %#v, want default sample values", analysis.Functions)
	}
}

func TestAnalyzeProfileUsesConventionalHeapSampleTypeWhenDefaultIsAbsent(t *testing.T) {
	profile := capturedTestProfile(t, "heap", 3, 1024)
	parsed, err := profilepkg.ParseData(profile.Data)
	if err != nil {
		t.Fatalf("parse test profile: %v", err)
	}
	parsed.DefaultSampleType = ""
	parsed.PeriodType = &profilepkg.ValueType{Type: "heap", Unit: "bytes"}
	var data bytes.Buffer
	if err := parsed.Write(&data); err != nil {
		t.Fatalf("write test profile: %v", err)
	}
	profile.Data = data.Bytes()
	service := NewService(fakeTargetRepository{}, reviewProfileStore{profiles: map[string]domain.CapturedProfile{"heap": profile}})

	analysis, err := service.AnalyzeProfile(context.Background(), "heap", 20)
	if err != nil {
		t.Fatalf("analyze profile: %v", err)
	}
	if analysis.SampleType != "inuse_space" || analysis.Total != 1024 {
		t.Fatalf("analysis = %#v, want inuse_space total 1024", analysis)
	}
}

func TestAnalyzeProfileUsesConventionalAllocationSampleTypeWhenDefaultIsAbsent(t *testing.T) {
	profile := capturedTestProfile(t, "allocs", 3, 1024)
	parsed, err := profilepkg.ParseData(profile.Data)
	if err != nil {
		t.Fatalf("parse test profile: %v", err)
	}
	parsed.SampleType = []*profilepkg.ValueType{{Type: "alloc_space", Unit: "bytes"}, {Type: "inuse_space", Unit: "bytes"}}
	parsed.Sample[0].Value = []int64{4096, 1024}
	parsed.DefaultSampleType = ""
	parsed.PeriodType = &profilepkg.ValueType{Type: "allocs", Unit: "bytes"}
	var data bytes.Buffer
	if err := parsed.Write(&data); err != nil {
		t.Fatalf("write test profile: %v", err)
	}
	profile.Data = data.Bytes()
	service := NewService(fakeTargetRepository{}, reviewProfileStore{profiles: map[string]domain.CapturedProfile{"allocs": profile}})

	analysis, err := service.AnalyzeProfile(context.Background(), "allocs", 20)
	if err != nil {
		t.Fatalf("analyze profile: %v", err)
	}
	if analysis.SampleType != "alloc_space" || analysis.Total != 4096 {
		t.Fatalf("analysis = %#v, want alloc_space total 4096", analysis)
	}
}

func TestDiffProfilesUsesDefaultSampleType(t *testing.T) {
	service := NewService(fakeTargetRepository{}, reviewProfileStore{profiles: map[string]domain.CapturedProfile{
		"before": capturedTestProfile(t, "before", 3, 1024),
		"after":  capturedTestProfile(t, "after", 1, 4096),
	}})

	diff, err := service.DiffProfiles(context.Background(), "before", "after", 20)
	if err != nil {
		t.Fatalf("diff profiles: %v", err)
	}
	if diff.SampleType != "inuse_space" || diff.Unit != "bytes" {
		t.Fatalf("sample = %s/%s, want inuse_space/bytes", diff.SampleType, diff.Unit)
	}
	if len(diff.Functions) != 1 || diff.Functions[0].Flat != 3072 || diff.Functions[0].Cumulative != 3072 {
		t.Fatalf("functions = %#v, want default sample delta", diff.Functions)
	}
}

func TestDiffProfilesUsesEachProfileSelectedSampleIndex(t *testing.T) {
	before := capturedTestProfile(t, "before", 3, 1024)
	after := capturedTestProfile(t, "after", 3, 4096)
	before.Data = profileWithDefaultSampleOrder(t, before.Data, []int{0, 1}, "inuse_space")
	after.Data = profileWithDefaultSampleOrder(t, after.Data, []int{1, 0}, "inuse_space")
	service := NewService(fakeTargetRepository{}, reviewProfileStore{profiles: map[string]domain.CapturedProfile{
		"before": before,
		"after":  after,
	}})

	diff, err := service.DiffProfiles(context.Background(), "before", "after", 20)
	if err != nil {
		t.Fatalf("diff profiles: %v", err)
	}
	if diff.SampleType != "inuse_space" || len(diff.Functions) != 1 || diff.Functions[0].Flat != 3072 {
		t.Fatalf("diff = %#v, want 3072 inuse_space delta", diff)
	}
}

func TestGenerateFlamegraphReturnsSVG(t *testing.T) {
	service := NewService(fakeTargetRepository{}, reviewProfileStore{profiles: map[string]domain.CapturedProfile{
		"heap": capturedTestProfile(t, "heap", 3, 1024),
	}})

	image, err := service.GenerateFlamegraph(context.Background(), "heap")
	if err != nil {
		t.Fatalf("generate flamegraph: %v", err)
	}
	if !bytes.HasPrefix(image, []byte("<svg")) || !bytes.Contains(image, []byte("example.com/service.handle")) {
		t.Fatalf("flamegraph = %q, want SVG with profile function", image)
	}
}

func TestDiagnoseUsesGoroutineDumpCountWhenRuntimeMetricIsNotPublished(t *testing.T) {
	service := NewService(fakeTargetRepository{
		goroutines: []byte(blockedRequests),
		expvars:    []byte(`{"memstats":{"HeapAlloc":1234}}`),
	}, memoryProfileStore{})

	report, err := service.Diagnose(context.Background(), "http://service:6060")
	if err != nil {
		t.Fatalf("diagnose: %v", err)
	}
	if report.Runtime.Goroutines != 3 {
		t.Fatalf("goroutines = %d, want 3 from goroutine dump", report.Runtime.Goroutines)
	}
	if report.Runtime.GoroutineSource != "goroutine_dump" {
		t.Fatalf("goroutine source = %q, want goroutine_dump", report.Runtime.GoroutineSource)
	}
}

func TestDiagnosePreservesPublishedZeroGoroutineMetric(t *testing.T) {
	service := NewService(fakeTargetRepository{
		goroutines: []byte(blockedRequests),
		expvars:    []byte(`{"num_goroutine":0}`),
	}, memoryProfileStore{})

	report, err := service.Diagnose(context.Background(), "http://service:6060")
	if err != nil {
		t.Fatalf("diagnose: %v", err)
	}
	if report.Runtime.Goroutines != 0 || report.Runtime.GoroutineSource != "expvar:num_goroutine" {
		t.Fatalf("runtime = %#v, want published metric preserved", report.Runtime)
	}
}

func TestExpvarParsingPreservesLargeIntegerCounters(t *testing.T) {
	vars, err := parseExpvars([]byte(`{
  "memstats": {"LastGC": 1700000000000000123},
  "gaz_diagnostics": {"db_pools": {"primary": {
    "wait_count": 9007199254740993,
    "wait_duration_ns": 9007199254740993
  }}}
}`))
	if err != nil {
		t.Fatalf("parse expvars: %v", err)
	}
	stats := runtimeStatsFromExpvars(vars)
	pools := dbPoolStatsFromExpvars(vars)
	if stats.LastGC.UnixNano() != 1700000000000000123 {
		t.Fatalf("last GC = %d, want exact nanosecond value", stats.LastGC.UnixNano())
	}
	if len(pools) != 1 || pools[0].WaitCount != 9007199254740993 || pools[0].WaitDuration != time.Duration(9007199254740993) {
		t.Fatalf("pools = %#v, want exact counter values", pools)
	}
}

func TestDBPoolStatsAreSortedByName(t *testing.T) {
	vars, err := parseExpvars([]byte(`{
  "gaz_diagnostics": {"db_pools": {
    "zeta": {"in_use": 1},
    "alpha": {"in_use": 2}
  }}
}`))
	if err != nil {
		t.Fatalf("parse expvars: %v", err)
	}
	pools := dbPoolStatsFromExpvars(vars)
	if len(pools) != 2 || pools[0].Name != "alpha" || pools[1].Name != "zeta" {
		t.Fatalf("pools = %#v, want alphabetical order", pools)
	}
}

type reviewProfileStore struct {
	profiles map[string]domain.CapturedProfile
}

func (s reviewProfileStore) Save(_ context.Context, profile domain.CapturedProfile) (domain.ProfileMetadata, error) {
	return profile.Metadata, nil
}

func (s reviewProfileStore) Load(_ context.Context, id string) (domain.CapturedProfile, error) {
	profile, ok := s.profiles[id]
	if !ok {
		return domain.CapturedProfile{}, fmt.Errorf("profile %q not found", id)
	}
	return profile, nil
}

func capturedTestProfile(t *testing.T, id string, allocObjects, inUseSpace int64) domain.CapturedProfile {
	t.Helper()
	function := &profilepkg.Function{ID: 1, Name: "example.com/service.handle"}
	location := &profilepkg.Location{ID: 1, Line: []profilepkg.Line{{Function: function}}}
	parsed := &profilepkg.Profile{
		SampleType:        []*profilepkg.ValueType{{Type: "alloc_objects", Unit: "count"}, {Type: "inuse_space", Unit: "bytes"}},
		DefaultSampleType: "inuse_space",
		Function:          []*profilepkg.Function{function},
		Location:          []*profilepkg.Location{location},
		Sample:            []*profilepkg.Sample{{Location: []*profilepkg.Location{location}, Value: []int64{allocObjects, inUseSpace}}},
	}
	var data bytes.Buffer
	if err := parsed.Write(&data); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	return domain.CapturedProfile{
		Metadata: domain.ProfileMetadata{ID: id, Type: "heap"},
		Data:     data.Bytes(),
	}
}

func profileWithDefaultSampleOrder(t *testing.T, data []byte, order []int, defaultType string) []byte {
	t.Helper()
	parsed, err := profilepkg.ParseData(data)
	if err != nil {
		t.Fatalf("parse test profile: %v", err)
	}
	sampleTypes := make([]*profilepkg.ValueType, len(order))
	for index, source := range order {
		sampleTypes[index] = parsed.SampleType[source]
	}
	for _, sample := range parsed.Sample {
		values := make([]int64, len(order))
		for index, source := range order {
			values[index] = sample.Value[source]
		}
		sample.Value = values
	}
	parsed.SampleType = sampleTypes
	parsed.DefaultSampleType = defaultType
	var output bytes.Buffer
	if err := parsed.Write(&output); err != nil {
		t.Fatalf("write reordered profile: %v", err)
	}
	return output.Bytes()
}
