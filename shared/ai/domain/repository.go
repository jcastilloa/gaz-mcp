package domain

import "context"

type AIRepository interface {
	GetAIResponse(ctx context.Context, req *Request) (*Response, error)
}
