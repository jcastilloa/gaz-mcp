package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestAnalyzeProfileReturnsPercentagesRatesAndInlineFramesWithoutRecursiveDoubleCount(t *testing.T) {
	leaf := &profilepkg.Function{ID: 1, Name: "example.com/service.leaf"}
	inlined := &profilepkg.Function{ID: 2, Name: "example.com/service.inlined"}
	leafLocation := &profilepkg.Location{ID: 1, Line: []profilepkg.Line{{Function: leaf}, {Function: inlined}}}
	recursiveLocation := &profilepkg.Location{ID: 2, Line: []profilepkg.Line{{Function: leaf}}}
	parsed := &profilepkg.Profile{
		SampleType:        []*profilepkg.ValueType{{Type: "cpu", Unit: "nanoseconds"}},
		DefaultSampleType: "cpu",
		DurationNanos:     int64(2 * time.Second),
		Function:          []*profilepkg.Function{leaf, inlined},
		Location:          []*profilepkg.Location{leafLocation, recursiveLocation},
		Sample:            []*profilepkg.Sample{{Location: []*profilepkg.Location{leafLocation, recursiveLocation}, Value: []int64{100}}},
	}
	analysis := summarizeProfile(domain.ProfileMetadata{}, parsed, 20)
	if !analysis.RatesAvailable || analysis.DurationNs != int64(2*time.Second) || analysis.TotalPerSecond != 50 {
		t.Fatalf("analysis timing = %#v, want 2s and 50/s", analysis)
	}
	leafSample := functionByName(t, analysis.Functions, leaf.Name)
	if leafSample.Flat != 100 || leafSample.Cumulative != 100 || leafSample.FlatPercent != 100 || leafSample.FlatPerSecond != 50 {
		t.Fatalf("leaf sample = %#v", leafSample)
	}
	inlineSample := functionByName(t, analysis.Functions, inlined.Name)
	if inlineSample.Flat != 0 || inlineSample.Cumulative != 100 || inlineSample.CumulativePercent != 100 {
		t.Fatalf("inline sample = %#v", inlineSample)
	}
}

func TestMetricSnapshotOmitsUnavailableTargetTimestamp(t *testing.T) {
	snapshot := metricSnapshotFromExpvarsAt(map[string]any{}, time.Unix(200, 0))
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("collected_at")) {
		t.Fatalf("snapshot = %s, missing target timestamp must be omitted", encoded)
	}
}

func TestDiffProfilesIncludesDurationNormalizedDeltas(t *testing.T) {
	before := capturedTimedProfile(t, "before", time.Second, 100)
	after := capturedTimedProfile(t, "after", 2*time.Second, 200)
	service := NewService(fakeTargetRepository{}, reviewProfileStore{profiles: map[string]domain.CapturedProfile{"before": before, "after": after}})
	diff, err := service.DiffProfiles(context.Background(), "before", "after", 20)
	if err != nil {
		t.Fatalf("diff profiles: %v", err)
	}
	if diff.DurationANs != int64(time.Second) || diff.DurationBNs != int64(2*time.Second) {
		t.Fatalf("durations = %d/%d", diff.DurationANs, diff.DurationBNs)
	}
	if !diff.RatesAvailable {
		t.Fatal("rates_available = false, want true")
	}
	if len(diff.Functions) != 1 || diff.Functions[0].Cumulative != 100 || diff.Functions[0].CumulativePerSecond != 0 {
		t.Fatalf("functions = %#v, want raw growth but unchanged rate", diff.Functions)
	}
}

func TestCaptureProfileRejectsIncompatibleOptions(t *testing.T) {
	service := NewService(fakeTargetRepository{}, memoryProfileStore{})
	for _, request := range []domain.ProfileRequest{
		{Target: "http://service:6060", Type: "cpu", ForceGC: true},
		{Target: "http://service:6060", Type: "heap", Seconds: 1, ForceGC: true},
	} {
		if _, err := service.CaptureProfile(context.Background(), request); err == nil {
			t.Errorf("request %#v should be rejected", request)
		}
	}
}

func TestDiffProfilesOrdersLargestAbsoluteChangeFirst(t *testing.T) {
	first := ProfileAnalysis{Functions: []FunctionSample{{Name: "large-decrease", Cumulative: 100}, {Name: "small-increase", Cumulative: 1}}}
	second := ProfileAnalysis{Functions: []FunctionSample{{Name: "small-increase", Cumulative: 11}}}
	diff := diffProfileSummaries(first, second, 1)
	if len(diff.Functions) != 1 || diff.Functions[0].Name != "large-decrease" {
		t.Fatalf("functions = %#v, want largest absolute change", diff.Functions)
	}
}

func TestDiffProfilesOrdersByNormalizedChangeWhenRatesAreAvailable(t *testing.T) {
	first := ProfileAnalysis{DurationNs: int64(time.Second), Functions: []FunctionSample{
		{Name: "large-rate-change", Cumulative: 100, CumulativePerSecond: 100},
	}}
	second := ProfileAnalysis{DurationNs: int64(2 * time.Second), Functions: []FunctionSample{
		{Name: "large-rate-change", Cumulative: 110, CumulativePerSecond: 55},
		{Name: "larger-raw-change", Cumulative: 20, CumulativePerSecond: 10},
	}}
	diff := diffProfileSummaries(first, second, 1)
	if len(diff.Functions) != 1 || diff.Functions[0].Name != "large-rate-change" {
		t.Fatalf("functions = %#v, want largest normalized change", diff.Functions)
	}
}

func TestAnalyzeProfileOrdersByAbsoluteCostForDeltaProfiles(t *testing.T) {
	functions := profileFunctions(&profilepkg.Profile{Sample: []*profilepkg.Sample{
		profileSample("large-decrease", -100),
		profileSample("small-increase", 10),
	}}, 0)
	if len(functions) != 2 || functions[0].Name != "large-decrease" {
		t.Fatalf("functions = %#v, want largest absolute cost first", functions)
	}
}

func TestParseExpvarsRejectsTrailingJSON(t *testing.T) {
	if _, err := parseExpvars([]byte(`{} {}`)); err == nil {
		t.Fatal("trailing JSON must be rejected")
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
	if report.Goroutines.Total != 3 {
		t.Fatalf("goroutine dump total = %d, want 3", report.Goroutines.Total)
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
	if numberAt(report.Metrics.After.Runtime, "goroutines", "total") != 0 {
		t.Fatalf("runtime metrics = %#v, want published zero preserved", report.Metrics.After.Runtime)
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

func capturedTimedProfile(t *testing.T, id string, duration time.Duration, value int64) domain.CapturedProfile {
	t.Helper()
	function := &profilepkg.Function{ID: 1, Name: "example.com/service.handle"}
	location := &profilepkg.Location{ID: 1, Line: []profilepkg.Line{{Function: function}}}
	parsed := &profilepkg.Profile{
		SampleType: []*profilepkg.ValueType{{Type: "cpu", Unit: "nanoseconds"}}, DefaultSampleType: "cpu",
		DurationNanos: duration.Nanoseconds(), Function: []*profilepkg.Function{function}, Location: []*profilepkg.Location{location},
		Sample: []*profilepkg.Sample{{Location: []*profilepkg.Location{location}, Value: []int64{value}}},
	}
	var data bytes.Buffer
	if err := parsed.Write(&data); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	return domain.CapturedProfile{Metadata: domain.ProfileMetadata{ID: id, Type: "cpu"}, Data: data.Bytes()}
}

func functionByName(t *testing.T, functions []FunctionSample, name string) FunctionSample {
	t.Helper()
	for _, function := range functions {
		if function.Name == name {
			return function
		}
	}
	t.Fatalf("function %q not found in %#v", name, functions)
	return FunctionSample{}
}

func profileSample(name string, value int64) *profilepkg.Sample {
	function := &profilepkg.Function{Name: name}
	location := &profilepkg.Location{Line: []profilepkg.Line{{Function: function}}}
	return &profilepkg.Sample{Location: []*profilepkg.Location{location}, Value: []int64{value}}
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
