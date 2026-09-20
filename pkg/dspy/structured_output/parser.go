package structured_output

import (
	"context"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// Parser defines the interface for structured output parsers.
// This follows the same pattern as dspy-go's XML interceptor but is format-agnostic.
// Config is passed as interface{} to avoid import cycles - each parser converts it to their own config type.
type Parser interface {
	// GenerateInstructions creates formatting instructions for the LLM based on the signature.
	// This is used in the format phase (pre-processing) to inject instructions into inputs.
	// config should be of type Config, but is interface{} to avoid import cycles.
	GenerateInstructions(signature core.Signature, config interface{}) (string, error)

	// InjectInstructions adds formatting instructions to the appropriate input field.
	// This is used in the format phase (pre-processing).
	InjectInstructions(inputs map[string]any, instructions string, signature core.Signature) error

	// FindResponseText locates the text content to parse from outputs.
	// This is used in the parse phase (post-processing) to find the raw response.
	FindResponseText(outputs map[string]any) string

	// ExtractContent extracts structured content from potentially mixed text.
	// This handles cases where structured output is embedded in markdown or other text.
	ExtractContent(responseText string) string

	// ParseOutputs parses structured output from raw outputs into structured fields.
	// This is used in the parse phase (post-processing).
	// Returns parsed fields merged with original outputs (with __raw_response removed).
	// config should be of type Config, but is interface{} to avoid import cycles.
	ParseOutputs(ctx context.Context, outputs map[string]any, signature core.Signature, config interface{}) (map[string]any, error)

	// FormatName returns the format name for logging/debugging.
	FormatName() string
}
