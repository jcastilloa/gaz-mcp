package image

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"

	domain "github.com/jcastillo/gaz-mcp/mcp/domain/image"
)

type Service struct {
	repository domain.Repository
	models     []domain.Model
	allowed    map[string]struct{}
}

func NewService(models []string, repository domain.Repository) Service {
	allowed := make(map[string]struct{}, len(models))
	configured := make([]domain.Model, 0, len(models))
	for _, model := range models {
		id := strings.TrimSpace(model)
		if id == "" {
			continue
		}
		if _, exists := allowed[id]; exists {
			continue
		}
		allowed[id] = struct{}{}
		configured = append(configured, domain.Model{ID: id})
	}
	return Service{repository: repository, models: configured, allowed: allowed}
}

func (s Service) ListModels() []domain.Model {
	return append([]domain.Model(nil), s.models...)
}

func (s Service) Generate(ctx context.Context, request domain.GenerateRequest) (domain.GenerateResponse, error) {
	request, err := s.normalizeRequest(request)
	if err != nil {
		return domain.GenerateResponse{}, err
	}
	return s.repository.Generate(ctx, request)
}

func (s Service) normalizeRequest(request domain.GenerateRequest) (domain.GenerateRequest, error) {
	request.Model = strings.TrimSpace(request.Model)
	if _, allowed := s.allowed[request.Model]; !allowed {
		return domain.GenerateRequest{}, fmt.Errorf("model %q is not configured", request.Model)
	}

	request.Prompt = strings.TrimSpace(request.Prompt)
	if request.Prompt == "" {
		return domain.GenerateRequest{}, errors.New("prompt is required")
	}

	if request.N == 0 {
		request.N = 1
	}
	if request.N < 1 || request.N > 10 {
		return domain.GenerateRequest{}, errors.New("n must be between 1 and 10")
	}

	request.Size = strings.TrimSpace(request.Size)
	request.Resolution = strings.TrimSpace(request.Resolution)
	request.AspectRatio = strings.TrimSpace(request.AspectRatio)
	if request.Size != "" && (request.Resolution != "" || request.AspectRatio != "") {
		return domain.GenerateRequest{}, errors.New("size cannot be combined with resolution or aspect_ratio")
	}
	if request.OutputCompression != nil && (*request.OutputCompression < 0 || *request.OutputCompression > 100) {
		return domain.GenerateRequest{}, errors.New("output_compression must be between 0 and 100")
	}
	if err := validateReferences(request.InputReferences); err != nil {
		return domain.GenerateRequest{}, err
	}
	return request, nil
}

func validateReferences(references []domain.InputReference) error {
	if len(references) > 10 {
		return errors.New("at most 10 input references are allowed")
	}
	for index, reference := range references {
		hasURL := strings.TrimSpace(reference.URL) != ""
		hasData := strings.TrimSpace(reference.Data) != ""
		if hasURL == hasData {
			return fmt.Errorf("input reference %d must contain exactly one of url or data", index+1)
		}
		if hasURL {
			parsed, err := url.Parse(reference.URL)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return fmt.Errorf("input reference %d URL must use http or https", index+1)
			}
			continue
		}
		if strings.TrimSpace(reference.MediaType) == "" {
			return fmt.Errorf("input reference %d media_type is required with data", index+1)
		}
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(reference.MediaType)), "image/") {
			return fmt.Errorf("input reference %d media_type must be an image MIME type", index+1)
		}
		if _, err := base64.StdEncoding.DecodeString(reference.Data); err != nil {
			return fmt.Errorf("input reference %d data must be valid base64: %w", index+1, err)
		}
	}
	return nil
}
