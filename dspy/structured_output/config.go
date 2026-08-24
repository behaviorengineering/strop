package structured_output

import (
	"time"

	stroplog "github.com/behaviorengineering/strop/log"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// FormatInstructionsSupplement appends extra formatting instructions from module inputs.
type FormatInstructionsSupplement func(inputs map[string]any) string

// ParseSignatureAdjuster may narrow the parse signature from module inputs.
type ParseSignatureAdjuster func(info *core.ModuleInfo, inputs map[string]any, signature core.Signature) core.Signature

// AfterParseHook runs after a successful structured parse.
type AfterParseHook func(inputs map[string]any, parsedOutputs map[string]any, info *core.ModuleInfo)

// Format represents the structured output format.
type Format string

const (
	FormatXML  Format = "xml"
	FormatJSON Format = "json" // Future support.
)

// Config holds configuration for structured output parsing.
type Config struct {
	// Format specifies the output format (xml, json, etc.)
	Format

	// StrictParsing requires all output fields to be present.
	StrictParsing bool

	// FallbackToText enables text fallback for malformed structured output.
	FallbackToText bool

	// ValidateXML performs XML syntax validation before parsing (XML only).
	ValidateXML bool

	// MaxDepth limits nesting depth for security (default: 10).
	MaxDepth int

	// MaxSize limits response size in bytes (default: 1MB).
	MaxSize int64

	// Timeout for parsing operations.
	ParseTimeout time.Duration

	// CustomTags allows overriding default tag names for fields (XML only).
	CustomTags map[string]string

	// IncludeTypeHints adds type information to format instructions.
	IncludeTypeHints bool

	// PreserveWhitespace maintains whitespace in content.
	PreserveWhitespace bool

	// ExtraArrayFieldsAsNewlineString names additional Text output fields that use repeated XML
	// child elements but are parsed into a single newline-joined string (XML only). Built-in
	// defaults always include usage_situations; list field names that match signature outputs.
	ExtraArrayFieldsAsNewlineString []string

	// Logger for debug logging (optional).
	Logger stroplog.Logger

	// FormatInstructionsSupplement appends product-specific XML rules (optional).
	FormatInstructionsSupplement FormatInstructionsSupplement

	// AdjustParseSignature may filter the signature before parse (optional).
	AdjustParseSignature ParseSignatureAdjuster

	// AfterParse runs after a successful parse (optional).
	AfterParse AfterParseHook
}

// DefaultConfig returns Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Format:             FormatXML,
		StrictParsing:      false, // Allow missing fields for robustness.
		FallbackToText:     true,
		ValidateXML:        false, // Disable validation to handle unescaped entities.
		MaxDepth:           10,
		MaxSize:            1024 * 1024, // 1MB.
		ParseTimeout:       30 * time.Second,
		CustomTags:         make(map[string]string),
		IncludeTypeHints:   false,
		PreserveWhitespace: false,
	}
}

// WithStrictParsing sets strict parsing mode.
func (c Config) WithStrictParsing(strict bool) Config {
	c.StrictParsing = strict
	return c
}

// WithFallback enables/disables text fallback.
func (c Config) WithFallback(fallback bool) Config {
	c.FallbackToText = fallback
	return c
}

// WithValidation enables/disables XML validation.
func (c Config) WithValidation(validate bool) Config {
	c.ValidateXML = validate
	return c
}

// WithMaxDepth sets maximum nesting depth.
func (c Config) WithMaxDepth(depth int) Config {
	c.MaxDepth = depth
	return c
}

// WithMaxSize sets maximum response size.
func (c Config) WithMaxSize(size int64) Config {
	c.MaxSize = size
	return c
}

// WithTimeout sets parsing timeout.
func (c Config) WithTimeout(timeout time.Duration) Config {
	c.ParseTimeout = timeout
	return c
}

// WithCustomTag sets a custom tag for a field (XML only).
func (c Config) WithCustomTag(fieldName, tagName string) Config {
	if c.CustomTags == nil {
		c.CustomTags = make(map[string]string)
	}
	c.CustomTags[fieldName] = tagName
	return c
}

// WithTypeHints enables/disables type hints in format instructions.
func (c Config) WithTypeHints(hints bool) Config {
	c.IncludeTypeHints = hints
	return c
}

// WithPreserveWhitespace enables/disables whitespace preservation.
func (c Config) WithPreserveWhitespace(preserve bool) Config {
	c.PreserveWhitespace = preserve
	return c
}

// WithLogger sets the logger for debug logging.
func (c Config) WithLogger(logger stroplog.Logger) Config {
	c.Logger = logger
	return c
}

// WithFormatInstructionsSupplement sets optional extra format instructions.
func (c Config) WithFormatInstructionsSupplement(fn FormatInstructionsSupplement) Config {
	c.FormatInstructionsSupplement = fn
	return c
}

// WithAdjustParseSignature sets optional signature filtering before parse.
func (c Config) WithAdjustParseSignature(fn ParseSignatureAdjuster) Config {
	c.AdjustParseSignature = fn
	return c
}

// WithAfterParse sets an optional hook after a successful parse.
func (c Config) WithAfterParse(fn AfterParseHook) Config {
	c.AfterParse = fn
	return c
}

// WithExtraArrayFieldsAsNewlineString appends field names for XML array-as-newline-string parsing (see ExtraArrayFieldsAsNewlineString).
func (c Config) WithExtraArrayFieldsAsNewlineString(names ...string) Config {
	c.ExtraArrayFieldsAsNewlineString = append(append([]string(nil), c.ExtraArrayFieldsAsNewlineString...), names...)
	return c
}

// GetTagName returns the tag name for a field (XML only).
func (c Config) GetTagName(fieldName string) string {
	if tag, exists := c.CustomTags[fieldName]; exists {
		return tag
	}
	return fieldName
}

// Preset configurations.

// StrictConfig creates a configuration with strict parsing requirements.
func StrictConfig() Config {
	return DefaultConfig().
		WithStrictParsing(true).
		WithFallback(false).
		WithValidation(true)
}

// FlexibleConfig creates a configuration with flexible parsing (allows fallback).
func FlexibleConfig() Config {
	return DefaultConfig().
		WithStrictParsing(false).
		WithFallback(true).
		WithValidation(false)
}
