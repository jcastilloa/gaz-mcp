package diagnostics

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/google/pprof/profile"
	domain "github.com/jcastillo/gaz-mcp/mcp/domain/diagnostics"
)

const (
	DefaultCPUProfileSeconds = 10
	MaxCPUProfileSeconds     = 60
	diagnosticSampleWindow   = time.Second
)

var validProfileTypes = map[string]bool{
	"cpu": true, "heap": true, "allocs": true, "goroutine": true,
	"block": true, "mutex": true, "threadcreate": true,
}

type Service struct {
	targets  domain.TargetRepository
	profiles domain.ProfileStore
	wait     func(context.Context, time.Duration) error
	now      func() time.Time
}

func NewService(targets domain.TargetRepository, profiles domain.ProfileStore) Service {
	return Service{targets: targets, profiles: profiles, wait: waitForSample, now: func() time.Time { return time.Now().UTC() }}
}

func (s Service) CaptureProfile(ctx context.Context, request domain.ProfileRequest) (domain.ProfileMetadata, error) {
	if !validProfileTypes[request.Type] {
		return domain.ProfileMetadata{}, fmt.Errorf("unsupported profile type %q", request.Type)
	}
	seconds, err := normalizeCaptureDuration(request.Type, request.Seconds)
	if err != nil {
		return domain.ProfileMetadata{}, err
	}
	if request.ForceGC && request.Type != "heap" {
		return domain.ProfileMetadata{}, fmt.Errorf("force GC is only supported for heap profiles")
	}
	if request.ForceGC && seconds > 0 {
		return domain.ProfileMetadata{}, fmt.Errorf("force GC cannot be combined with a delta profile")
	}
	request.Seconds = seconds
	result, err := s.targets.FetchProfile(ctx, request)
	if err != nil {
		return domain.ProfileMetadata{}, fmt.Errorf("fetch profile: %w", err)
	}
	return s.profiles.Save(ctx, domain.CapturedProfile{Metadata: domain.ProfileMetadata{Target: request.Target, Type: request.Type, CapturedAt: s.now(), Alerts: result.Alerts}, Data: result.Data})
}

func (s Service) AnalyzeProfile(ctx context.Context, id string, topN int) (ProfileAnalysis, error) {
	loaded, err := s.loadProfile(ctx, id)
	if err != nil {
		return ProfileAnalysis{}, err
	}
	return summarizeProfile(loaded.metadata, loaded.profile, topN), nil
}

func (s Service) GenerateFlamegraph(ctx context.Context, id string) ([]byte, error) {
	loaded, err := s.loadProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	return renderFlamegraph(loaded.profile)
}

func (s Service) DiffProfiles(ctx context.Context, idA, idB string, topN int) (ProfileDiff, error) {
	first, err := s.loadProfile(ctx, idA)
	if err != nil {
		return ProfileDiff{}, err
	}
	second, err := s.loadProfile(ctx, idB)
	if err != nil {
		return ProfileDiff{}, err
	}
	firstSample, ok := selectedSampleType(first.profile)
	if !ok {
		return ProfileDiff{}, fmt.Errorf("first profile has no sample type")
	}
	secondSample, ok := selectedSampleType(second.profile)
	if !ok {
		return ProfileDiff{}, fmt.Errorf("second profile has no sample type")
	}
	if firstSample.Type != secondSample.Type || firstSample.Unit != secondSample.Unit {
		return ProfileDiff{}, fmt.Errorf("profiles must use the same primary sample type")
	}
	firstSummary := summarizeProfileForSample(first.metadata, first.profile, firstSample, 0)
	secondSummary := summarizeProfileForSample(second.metadata, second.profile, secondSample, 0)
	return diffProfileSummaries(firstSummary, secondSummary, topN), nil
}

func (s Service) AnalyzeGoroutines(ctx context.Context, target string) (GoroutineReport, error) {
	result, err := s.targets.FetchGoroutines(ctx, target)
	if err != nil {
		return GoroutineReport{}, fmt.Errorf("fetch goroutines: %w", err)
	}
	report, err := AnalyzeGoroutines(string(result.Data))
	if err != nil {
		return GoroutineReport{}, err
	}
	report.Alerts = result.Alerts
	return report, nil
}

func (s Service) GetExpvars(ctx context.Context, target string) (map[string]any, error) {
	vars, _, err := s.GetExpvarsWithAlerts(ctx, target)
	return vars, err
}

func (s Service) GetExpvarsWithAlerts(ctx context.Context, target string) (map[string]any, []domain.InspectionAlert, error) {
	result, err := s.targets.FetchExpvars(ctx, target)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch expvars: %w", err)
	}
	vars, err := parseExpvars(result.Data)
	if err != nil {
		return nil, nil, err
	}
	return vars, result.Alerts, nil
}

func (s Service) GetRuntimeStats(ctx context.Context, target string) (RuntimeStats, error) {
	vars, alerts, err := s.GetExpvarsWithAlerts(ctx, target)
	if err != nil {
		return RuntimeStats{}, err
	}
	stats := runtimeStatsFromExpvars(vars)
	stats.Alerts = alerts
	return stats, nil
}

func (s Service) GetDBPoolStats(ctx context.Context, target string) ([]DBPoolStats, error) {
	pools, _, err := s.GetDBPoolStatsWithAlerts(ctx, target)
	return pools, err
}

func (s Service) GetDBPoolStatsWithAlerts(ctx context.Context, target string) ([]DBPoolStats, []domain.InspectionAlert, error) {
	vars, alerts, err := s.GetExpvarsWithAlerts(ctx, target)
	if err != nil {
		return nil, nil, err
	}
	return dbPoolStatsFromExpvars(vars), alerts, nil
}

func (s Service) Diagnose(ctx context.Context, target string) (DiagnosisReport, error) {
	goroutines, err := s.AnalyzeGoroutines(ctx, target)
	var availabilityAlerts []domain.InspectionAlert
	if err != nil {
		if ctx.Err() != nil {
			return DiagnosisReport{}, ctx.Err()
		}
		goroutines = GoroutineReport{Groups: []GoroutineGroup{}}
		availabilityAlerts = append(availabilityAlerts, domain.InspectionAlert{
			Code: "GOROUTINES_UNAVAILABLE", Severity: "info",
			Description: "The target goroutine profile could not be read; stack-based suspicions are unavailable.",
		})
	}
	beforeVars, beforeAlerts, err := s.GetExpvarsWithAlerts(ctx, target)
	if err != nil {
		if ctx.Err() != nil {
			return DiagnosisReport{}, ctx.Err()
		}
		report := DiagnosisReport{Goroutines: goroutines}
		report.Metrics = metricWindow(emptyMetricSnapshot(), emptyMetricSnapshot())
		report.Suspicions = analyzeMetricSnapshots(report.Metrics.Before, report.Metrics.After, goroutines)
		clearGoroutineFindings(&report)
		report.Alerts = uniqueAlerts(goroutines.Alerts, availabilityAlerts, []domain.InspectionAlert{{
			Code:        "METRICS_UNAVAILABLE",
			Severity:    "info",
			Description: "The target /debug/vars endpoint could not be read; metric-based suspicions are unavailable.",
		}})
		report.Goroutines.Alerts = nil
		return report, nil
	}
	before := metricSnapshotFromExpvarsAt(beforeVars, s.now())
	if err := s.wait(ctx, diagnosticSampleWindow); err != nil {
		return DiagnosisReport{}, err
	}
	afterVars, afterAlerts, err := s.GetExpvarsWithAlerts(ctx, target)
	if err != nil {
		if ctx.Err() != nil {
			return DiagnosisReport{}, ctx.Err()
		}
		after := before
		afterAlerts = append(afterAlerts, domain.InspectionAlert{
			Code:        "METRIC_SECOND_SAMPLE_UNAVAILABLE",
			Severity:    "info",
			Description: "The second metric sample could not be read; the first sample was reused and rate-based suspicions were skipped.",
		})
		report := DiagnosisReport{Goroutines: goroutines, Metrics: metricWindow(before, after)}
		report.Suspicions = analyzeMetricSnapshots(before, after, goroutines)
		clearGoroutineFindings(&report)
		report.Alerts = uniqueAlerts(goroutines.Alerts, availabilityAlerts, beforeAlerts, afterAlerts)
		report.Goroutines.Alerts = nil
		return report, nil
	}
	after := metricSnapshotFromExpvarsAt(afterVars, s.now())
	report := DiagnosisReport{Goroutines: goroutines}
	report.Metrics = metricWindow(before, after)
	report.Suspicions = analyzeMetricSnapshots(before, after, goroutines)
	clearGoroutineFindings(&report)
	report.Alerts = uniqueAlerts(goroutines.Alerts, availabilityAlerts, beforeAlerts, afterAlerts)
	report.Goroutines.Alerts = nil
	return report, nil
}

func waitForSample(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func uniqueAlerts(alertGroups ...[]domain.InspectionAlert) []domain.InspectionAlert {
	seen := make(map[string]struct{})
	var alerts []domain.InspectionAlert
	for _, group := range alertGroups {
		for _, alert := range group {
			key := alert.Code + "\x00" + alert.Description
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			alerts = append(alerts, alert)
		}
	}
	return alerts
}

type loadedProfile struct {
	metadata domain.ProfileMetadata
	profile  *profile.Profile
}

func (s Service) loadProfile(ctx context.Context, id string) (loadedProfile, error) {
	captured, err := s.profiles.Load(ctx, id)
	if err != nil {
		return loadedProfile{}, err
	}
	parsed, err := profile.ParseData(captured.Data)
	if err != nil {
		return loadedProfile{}, fmt.Errorf("parse profile %q: %w", id, err)
	}
	return loadedProfile{metadata: captured.Metadata, profile: parsed}, nil
}

func normalizeCaptureDuration(profileType string, seconds int) (int, error) {
	if seconds == 0 {
		if profileType == "cpu" {
			return DefaultCPUProfileSeconds, nil
		}
		return 0, nil
	}
	if seconds < 1 || seconds > MaxCPUProfileSeconds {
		return 0, fmt.Errorf("profile seconds must be between 1 and %d", MaxCPUProfileSeconds)
	}
	return seconds, nil
}

func summarizeProfile(metadata domain.ProfileMetadata, parsed *profile.Profile, topN int) ProfileAnalysis {
	sampleType, ok := selectedSampleType(parsed)
	if !ok {
		return ProfileAnalysis{Profile: metadata}
	}
	return summarizeProfileForSample(metadata, parsed, sampleType, topN)
}

func summarizeProfileForSample(metadata domain.ProfileMetadata, parsed *profile.Profile, sampleType sampleType, topN int) ProfileAnalysis {
	functions := profileFunctions(parsed, sampleType.index)
	total := profileTotal(parsed, sampleType.index)
	seconds := time.Duration(parsed.DurationNanos).Seconds()
	for index := range functions {
		if total != 0 {
			functions[index].FlatPercent = float64(functions[index].Flat) / float64(total) * 100
			functions[index].CumulativePercent = float64(functions[index].Cumulative) / float64(total) * 100
		}
		if seconds > 0 {
			functions[index].FlatPerSecond = float64(functions[index].Flat) / seconds
			functions[index].CumulativePerSecond = float64(functions[index].Cumulative) / seconds
		}
	}
	if topN > 0 && len(functions) > topN {
		functions = functions[:topN]
	}
	analysis := ProfileAnalysis{
		Profile: metadata, SampleType: sampleType.Type, Unit: sampleType.Unit,
		DurationNs: parsed.DurationNanos, RatesAvailable: seconds > 0, Total: total, Functions: functions,
	}
	if seconds > 0 {
		analysis.TotalPerSecond = float64(total) / seconds
	}
	return analysis
}

type sampleType struct {
	index int
	Type  string
	Unit  string
}

func selectedSampleType(parsed *profile.Profile) (sampleType, bool) {
	if len(parsed.SampleType) == 0 {
		return sampleType{}, false
	}
	for index, value := range parsed.SampleType {
		if value.Type == parsed.DefaultSampleType {
			return sampleType{index: index, Type: value.Type, Unit: value.Unit}, true
		}
	}
	if index := preferredSampleType(parsed); index >= 0 {
		value := parsed.SampleType[index]
		return sampleType{index: index, Type: value.Type, Unit: value.Unit}, true
	}
	value := parsed.SampleType[0]
	return sampleType{index: 0, Type: value.Type, Unit: value.Unit}, true
}

func preferredSampleType(parsed *profile.Profile) int {
	preferred := map[string]string{
		"heap":   "inuse_space",
		"allocs": "alloc_space",
	}
	wanted := preferred[parsed.PeriodType.Type]
	for index, value := range parsed.SampleType {
		if value.Type == wanted {
			return index
		}
	}
	return -1
}

func profileTotal(parsed *profile.Profile, sampleIndex int) int64 {
	var total int64
	for _, sample := range parsed.Sample {
		if sampleIndex < len(sample.Value) {
			total += sample.Value[sampleIndex]
		}
	}
	return total
}

func profileFunctions(parsed *profile.Profile, sampleIndex int) []FunctionSample {
	values := make(map[string]*FunctionSample)
	for _, sample := range parsed.Sample {
		if sampleIndex >= len(sample.Value) || len(sample.Location) == 0 {
			continue
		}
		value := sample.Value[sampleIndex]
		seen := make(map[string]struct{})
		flatAssigned := false
		for _, location := range sample.Location {
			for _, line := range location.Line {
				if line.Function == nil || line.Function.Name == "" {
					continue
				}
				name := line.Function.Name
				item := values[name]
				if item == nil {
					item = &FunctionSample{Name: name}
					values[name] = item
				}
				if _, duplicate := seen[name]; !duplicate {
					item.Cumulative += value
					seen[name] = struct{}{}
				}
				if !flatAssigned {
					item.Flat += value
					flatAssigned = true
				}
			}
		}
	}
	functions := make([]FunctionSample, 0, len(values))
	for _, value := range values {
		functions = append(functions, *value)
	}
	sort.Slice(functions, func(i, j int) bool {
		left, right := math.Abs(float64(functions[i].Cumulative)), math.Abs(float64(functions[j].Cumulative))
		if left == right {
			return functions[i].Name < functions[j].Name
		}
		return left > right
	})
	return functions
}

func diffProfileSummaries(first, second ProfileAnalysis, topN int) ProfileDiff {
	ratesAvailable := first.DurationNs > 0 && second.DurationNs > 0
	values := make(map[string]FunctionDelta, len(first.Functions)+len(second.Functions))
	for _, function := range second.Functions {
		values[function.Name] = FunctionDelta{Name: function.Name, Flat: function.Flat, Cumulative: function.Cumulative, FlatPerSecond: function.FlatPerSecond, CumulativePerSecond: function.CumulativePerSecond}
	}
	for _, function := range first.Functions {
		delta := values[function.Name]
		delta.Name = function.Name
		delta.Flat -= function.Flat
		delta.Cumulative -= function.Cumulative
		delta.FlatPerSecond -= function.FlatPerSecond
		delta.CumulativePerSecond -= function.CumulativePerSecond
		values[function.Name] = delta
	}
	functions := make([]FunctionDelta, 0, len(values))
	for _, value := range values {
		if value.Flat != 0 || value.Cumulative != 0 || value.FlatPerSecond != 0 || value.CumulativePerSecond != 0 {
			functions = append(functions, value)
		}
	}
	sort.Slice(functions, func(i, j int) bool {
		left, right := math.Abs(float64(functions[i].Cumulative)), math.Abs(float64(functions[j].Cumulative))
		if ratesAvailable {
			left, right = math.Abs(functions[i].CumulativePerSecond), math.Abs(functions[j].CumulativePerSecond)
		}
		if left == right {
			return functions[i].Name < functions[j].Name
		}
		return left > right
	})
	if topN > 0 && len(functions) > topN {
		functions = functions[:topN]
	}
	return ProfileDiff{
		ProfileA: first.Profile, ProfileB: second.Profile, SampleType: first.SampleType, Unit: first.Unit,
		DurationANs: first.DurationNs, DurationBNs: second.DurationNs,
		RatesAvailable: ratesAvailable, Functions: functions,
	}
}
