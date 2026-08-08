package diagnostics

import "context"

type TargetRepository interface {
	FetchProfile(ctx context.Context, request ProfileRequest) (FetchResult, error)
	FetchGoroutines(ctx context.Context, target string) (FetchResult, error)
	FetchExpvars(ctx context.Context, target string) (FetchResult, error)
}

type ProfileRequest struct {
	Target  string
	Type    string
	Seconds int
	ForceGC bool
}

type FetchResult struct {
	Data   []byte
	Alerts []InspectionAlert
}

type InspectionAlert struct {
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

type ProfileStore interface {
	Save(ctx context.Context, profile CapturedProfile) (ProfileMetadata, error)
	Load(ctx context.Context, id string) (CapturedProfile, error)
}
