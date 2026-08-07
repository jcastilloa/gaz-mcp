package diagnostics

import "time"

type CapturedProfile struct {
	Metadata ProfileMetadata
	Data     []byte
}

type ProfileMetadata struct {
	ID         string    `json:"id"`
	Target     string    `json:"target"`
	Type       string    `json:"type"`
	CapturedAt time.Time `json:"captured_at"`
	Size       int       `json:"size_bytes"`
}
