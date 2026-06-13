package domain

import (
	"fmt"
	"strings"
)

type ServiceConfig struct {
	Host      string
	Port      int
	APIPrefix string
	Version   string
	Transport string
}

func (c ServiceConfig) HTTPAddress() string {
	host := strings.TrimSpace(c.Host)
	if host == "" {
		host = "0.0.0.0"
	}
	port := c.Port
	if port <= 0 {
		port = 8080
	}
	return fmt.Sprintf("%s:%d", host, port)
}

func (c ServiceConfig) NormalizedAPIPrefix() string {
	prefix := strings.TrimSpace(c.APIPrefix)
	if prefix == "" {
		return "/v1"
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return strings.TrimRight(prefix, "/")
}

func (c ServiceConfig) NormalizedTransport() string {
	transport := strings.ToLower(strings.TrimSpace(c.Transport))
	if transport == "" {
		return "stdio"
	}
	return transport
}
