package diagnostics

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/pprof/profile"
	domain "github.com/jcastillo/gaz-mcp/mcp/domain/diagnostics"
)

const (
	DefaultCPUProfileSeconds = 10
	MaxCPUProfileSeconds     = 60
)

var validProfileTypes = map[string]bool{
	"cpu": true, "heap": true, "allocs": true, "goroutine": true,
	"block": true, "mutex": true, "threadcreate": true,
}

type Service struct {
	targets  domain.TargetRepository
	profiles domain.ProfileStore
}

func NewService(targets domain.TargetRepository, profiles domain.ProfileStore) Service {
	return Service{targets: targets, profiles: profiles}
}

func (s Service) CaptureProfile(ctx context.Context, target, profileType string, seconds int) (domain.ProfileMetadata, error) {
	if !validProfileTypes[profileType] {
		return domain.ProfileMetadata{}, fmt.Errorf("unsupported profile type %q", profileType)
	}
	seconds, err := normalizeCaptureDuration(profileType, seconds)
	if err != nil {
		return domain.ProfileMetadata{}, err
	}
	data, err := s.targets.FetchProfile(ctx, target, profileType, seconds)
	if err != nil {
		return domain.ProfileMetadata{}, fmt.Errorf("fetch profile: %w", err)
	}
	return s.profiles.Save(ctx, domain.CapturedProfile{Metadata: domain.ProfileMetadata{Target: target, Type: profileType, CapturedAt: time.Now().UTC()}, Data: data})
}

func (s Service) AnalyzeProfile(ctx context.Context, id string, topN int) (ProfileAnalysis, error) {
	captured, err := s.profiles.Load(ctx, id)
	if err != nil {
		return ProfileAnalysis{}, err
	}
	parsed, err := profile.ParseData(captured.Data)
	if err != nil {
		return ProfileAnalysis{}, fmt.Errorf("parse profile %q: %w", id, err)
	}
	return summarizeProfile(captured.Metadata, parsed, topN), nil
}

func (s Service) ProfileData(ctx context.Context, id string) ([]byte, error) {
	captured, err := s.profiles.Load(ctx, id)
	if err != nil {
		return nil, err
	}
	return captured.Data, nil
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
	if len(first.profile.SampleType) == 0 || len(second.profile.SampleType) == 0 || first.profile.SampleType[0].Type != second.profile.SampleType[0].Type || first.profile.SampleType[0].Unit != second.profile.SampleType[0].Unit {
		return ProfileDiff{}, fmt.Errorf("profiles must use the same primary sample type")
	}
	firstSummary := summarizeProfile(first.metadata, first.profile, 0)
	secondSummary := summarizeProfile(second.metadata, second.profile, 0)
	return diffProfileSummaries(firstSummary, secondSummary, topN), nil
}

func (s Service) AnalyzeGoroutines(ctx context.Context, target string) (GoroutineReport, error) {
	dump, err := s.targets.FetchGoroutines(ctx, target)
	if err != nil {
		return GoroutineReport{}, fmt.Errorf("fetch goroutines: %w", err)
	}
	return AnalyzeGoroutines(string(dump))
}

func (s Service) GetExpvars(ctx context.Context, target string) (map[string]any, error) {
	data, err := s.targets.FetchExpvars(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("fetch expvars: %w", err)
	}
	return parseExpvars(data)
}

func (s Service) GetRuntimeStats(ctx context.Context, target string) (RuntimeStats, error) {
	vars, err := s.GetExpvars(ctx, target)
	if err != nil {
		return RuntimeStats{}, err
	}
	return runtimeStatsFromExpvars(vars), nil
}

func (s Service) GetDBPoolStats(ctx context.Context, target string) ([]DBPoolStats, error) {
	vars, err := s.GetExpvars(ctx, target)
	if err != nil {
		return nil, err
	}
	return dbPoolStatsFromExpvars(vars), nil
}

func (s Service) Diagnose(ctx context.Context, target string) (DiagnosisReport, error) {
	goroutines, err := s.AnalyzeGoroutines(ctx, target)
	if err != nil {
		return DiagnosisReport{}, err
	}
	vars, err := s.GetExpvars(ctx, target)
	if err != nil {
		return BuildDiagnosis(RuntimeStats{Instrumentation: "unavailable: /debug/vars could not be read"}, goroutines, nil), nil
	}
	return BuildDiagnosis(runtimeStatsFromExpvars(vars), goroutines, dbPoolStatsFromExpvars(vars)), nil
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
	if profileType != "cpu" {
		return 0, nil
	}
	if seconds == 0 {
		return DefaultCPUProfileSeconds, nil
	}
	if seconds < 1 || seconds > MaxCPUProfileSeconds {
		return 0, fmt.Errorf("cpu profile seconds must be between 1 and %d", MaxCPUProfileSeconds)
	}
	return seconds, nil
}

func summarizeProfile(metadata domain.ProfileMetadata, parsed *profile.Profile, topN int) ProfileAnalysis {
	if len(parsed.SampleType) == 0 {
		return ProfileAnalysis{Profile: metadata}
	}
	functions := profileFunctions(parsed)
	if topN > 0 && len(functions) > topN {
		functions = functions[:topN]
	}
	return ProfileAnalysis{Profile: metadata, SampleType: parsed.SampleType[0].Type, Unit: parsed.SampleType[0].Unit, Total: profileTotal(parsed), Functions: functions}
}

func profileTotal(parsed *profile.Profile) int64 {
	var total int64
	for _, sample := range parsed.Sample {
		if len(sample.Value) > 0 {
			total += sample.Value[0]
		}
	}
	return total
}

func profileFunctions(parsed *profile.Profile) []FunctionSample {
	values := make(map[string]*FunctionSample)
	for _, sample := range parsed.Sample {
		if len(sample.Value) == 0 || len(sample.Location) == 0 {
			continue
		}
		value := sample.Value[0]
		for index, location := range sample.Location {
			name := locationName(location)
			if name == "" {
				continue
			}
			item := values[name]
			if item == nil {
				item = &FunctionSample{Name: name}
				values[name] = item
			}
			item.Cumulative += value
			if index == 0 {
				item.Flat += value
			}
		}
	}
	functions := make([]FunctionSample, 0, len(values))
	for _, value := range values {
		functions = append(functions, *value)
	}
	sort.Slice(functions, func(i, j int) bool {
		if functions[i].Cumulative == functions[j].Cumulative {
			return functions[i].Name < functions[j].Name
		}
		return functions[i].Cumulative > functions[j].Cumulative
	})
	return functions
}

func locationName(location *profile.Location) string {
	if len(location.Line) == 0 || location.Line[0].Function == nil {
		return ""
	}
	return location.Line[0].Function.Name
}

func diffProfileSummaries(first, second ProfileAnalysis, topN int) ProfileDiff {
	values := make(map[string]FunctionDelta, len(first.Functions)+len(second.Functions))
	for _, function := range second.Functions {
		values[function.Name] = FunctionDelta{Name: function.Name, Flat: function.Flat, Cumulative: function.Cumulative}
	}
	for _, function := range first.Functions {
		delta := values[function.Name]
		delta.Name = function.Name
		delta.Flat -= function.Flat
		delta.Cumulative -= function.Cumulative
		values[function.Name] = delta
	}
	functions := make([]FunctionDelta, 0, len(values))
	for _, value := range values {
		if value.Flat != 0 || value.Cumulative != 0 {
			functions = append(functions, value)
		}
	}
	sort.Slice(functions, func(i, j int) bool { return functions[i].Cumulative > functions[j].Cumulative })
	if topN > 0 && len(functions) > topN {
		functions = functions[:topN]
	}
	return ProfileDiff{ProfileA: first.Profile, ProfileB: second.Profile, SampleType: first.SampleType, Unit: first.Unit, Functions: functions}
}
