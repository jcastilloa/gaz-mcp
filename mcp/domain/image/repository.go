package image

import "context"

type Repository interface {
	Generate(ctx context.Context, request GenerateRequest) (GenerateResponse, error)
}
