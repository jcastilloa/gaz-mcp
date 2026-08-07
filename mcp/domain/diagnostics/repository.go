package diagnostics

import "context"

type TargetRepository interface {
	FetchProfile(ctx context.Context, target, profileType string, seconds int) ([]byte, error)
	FetchGoroutines(ctx context.Context, target string) ([]byte, error)
	FetchExpvars(ctx context.Context, target string) ([]byte, error)
}

type ProfileStore interface {
	Save(ctx context.Context, profile CapturedProfile) (ProfileMetadata, error)
	Load(ctx context.Context, id string) (CapturedProfile, error)
}
