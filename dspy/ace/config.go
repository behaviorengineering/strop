package ace

import "fmt"

// Config configures a session-scoped ACE Manager.
// Enabled and LearningsPath are required for NewManager.
// LearningsPath must be host-keyed (entity/job/repo); there is no usable default path.
type Config struct {
	Enabled           bool
	LearningsPath     string
	AsyncReflection   bool
	CurationFrequency int
	MinConfidence     float64
	MaxTokens         int
}

// Defaults fills curation knobs. AsyncReflection stays false unless the host sets it.
// Does not invent a LearningsPath.
func (c Config) Defaults() Config {
	if c.CurationFrequency <= 0 {
		c.CurationFrequency = 5
	}
	if c.MinConfidence <= 0 {
		c.MinConfidence = 0.6
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = 80000
	}
	return c
}

// Validate checks host-facing requirements before constructing a Manager.
func (c Config) Validate() error {
	c = c.Defaults()
	if !c.Enabled {
		return fmt.Errorf("ace: Enabled must be true to construct a Manager")
	}
	if c.LearningsPath == "" {
		return fmt.Errorf("ace: LearningsPath is required when Enabled (host must key by context)")
	}
	if c.MinConfidence < 0 || c.MinConfidence > 1 {
		return fmt.Errorf("ace: MinConfidence must be between 0 and 1")
	}
	if c.MaxTokens <= 0 {
		return fmt.Errorf("ace: MaxTokens must be positive")
	}
	if c.CurationFrequency <= 0 {
		return fmt.Errorf("ace: CurationFrequency must be positive")
	}
	return nil
}
