package dspy

import (
	"fmt"
	"time"
)

// GroundingConfig holds Google Gemini search grounding options.
type GroundingConfig struct {
	DynamicThreshold float64
}

// ProviderConfig is the portable AI provider DTO used by kit factories.
// Apps map YAML/env into this type at the container boundary.
type ProviderConfig struct {
	APIKey           string
	Model            string
	BaseURL          string
	Timeout          string
	RateLimit        int
	APISchema        string
	MaxContextTokens int
	MaxOutputTokens  int
	Grounding        *GroundingConfig
}

// GetTimeout returns the per-attempt HTTP/LLM timeout.
// When Timeout is unset or invalid, moduleTimeout is used.
func (p ProviderConfig) GetTimeout(moduleTimeout time.Duration) time.Duration {
	if p.Timeout == "" {
		return moduleTimeout
	}
	parsed, err := time.ParseDuration(p.Timeout)
	if err != nil {
		return moduleTimeout
	}
	return parsed
}

// Validate reports missing required provider fields.
func (p ProviderConfig) Validate() error {
	if p.APIKey == "" {
		return fmt.Errorf("api_key is required")
	}
	if p.Model == "" {
		return fmt.Errorf("model is required")
	}
	if p.BaseURL == "" {
		return fmt.Errorf("base_url is required")
	}
	if p.APISchema == "" {
		return fmt.Errorf("api_schema is required")
	}
	return nil
}
