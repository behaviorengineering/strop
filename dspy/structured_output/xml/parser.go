package xml

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/behaviorengineering/strop/dspy/rawresponse"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	stroplog "github.com/behaviorengineering/strop/log"
)

// XMLConfig represents XML-specific configuration.
// This is separate from structured_output.Config to avoid import cycles.
type XMLConfig struct {
	StrictParsing      bool
	FallbackToText     bool
	ValidateXML        bool
	MaxDepth           int
	MaxSize            int64
	ParseTimeout       time.Duration
	CustomTags         map[string]string
	IncludeTypeHints   bool
	PreserveWhitespace bool
	// ExtraArrayFieldsAsNewlineString lists additional Text fields parsed as repeated XML children
	// but stored as one newline-joined string. Built-in defaults always include usage_situations.
	ExtraArrayFieldsAsNewlineString []string
	Logger                          stroplog.Logger
}

// GetTagName returns the XML tag name for a field.
func (c XMLConfig) GetTagName(fieldName string) string {
	if tag, exists := c.CustomTags[fieldName]; exists {
		return tag
	}
	return fieldName
}

const (
	// maxCacheSize limits the number of cached signatures to prevent unbounded memory growth.
	maxCacheSize = 100
)

// XMLParser implements the Parser interface for XML format.
// This follows dspy-go's XML interceptor pattern but adds support for nested XML elements.
type XMLParser struct {
	cache      map[string]*ParsedSignature
	cacheOrder []string // LRU order tracking.
	mutex      sync.RWMutex
}

// NewXMLParser creates a new XML parser.
func NewXMLParser() *XMLParser {
	return &XMLParser{
		cache:      make(map[string]*ParsedSignature),
		cacheOrder: make([]string, 0, maxCacheSize),
	}
}

// ParsedSignature caches parsing information for a signature.
type ParsedSignature struct {
	FieldMap    map[string]core.OutputField
	TagMap      map[string]string // tag -> field name.
	RequiredSet map[string]bool
	// MapFields tracks which fields are expected to be maps (for nested XML parsing).
	MapFields map[string]bool
	// ArrayFields tracks which fields are expected to be arrays (repeated child elements, e.g. sub_questions with <question> items).
	ArrayFields map[string]bool
	// NewlineJoinedArrayFields marks array-parsed Text fields flushed as a single newline-joined string.
	NewlineJoinedArrayFields map[string]bool
}

// FormatName returns the format name.
func (p *XMLParser) FormatName() string {
	return "xml"
}

// convertToXMLConfig converts a structured_output.Config to XMLConfig using reflection.
// This avoids import cycles by not importing structured_output package.
func convertToXMLConfig(config interface{}) XMLConfig {
	// Use reflection to extract config fields.
	v := reflect.ValueOf(config)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		// Fallback to defaults if conversion fails.
		return XMLConfig{
			StrictParsing:                   true,
			FallbackToText:                  false,
			ValidateXML:                     false,
			MaxDepth:                        10,
			MaxSize:                         1024 * 1024,
			ParseTimeout:                    30 * time.Second,
			CustomTags:                      make(map[string]string),
			IncludeTypeHints:                false,
			PreserveWhitespace:              false,
			ExtraArrayFieldsAsNewlineString: nil,
		}
	}

	xmlConfig := XMLConfig{
		// Initialize with defaults in case some fields are missing.
		StrictParsing:                   true,
		FallbackToText:                  false,
		ValidateXML:                     false,
		MaxDepth:                        10,
		MaxSize:                         1024 * 1024,
		ParseTimeout:                    30 * time.Second,
		CustomTags:                      make(map[string]string),
		IncludeTypeHints:                false,
		PreserveWhitespace:              false,
		ExtraArrayFieldsAsNewlineString: nil,
	}
	t := v.Type()

	// Extract fields by name using reflection.
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		switch field.Name {
		case "StrictParsing":
			if value.Kind() == reflect.Bool {
				xmlConfig.StrictParsing = value.Bool()
			}
		case "FallbackToText":
			if value.Kind() == reflect.Bool {
				xmlConfig.FallbackToText = value.Bool()
			}
		case "ValidateXML":
			if value.Kind() == reflect.Bool {
				xmlConfig.ValidateXML = value.Bool()
			}
		case "MaxDepth":
			if value.Kind() == reflect.Int {
				xmlConfig.MaxDepth = int(value.Int())
			}
		case "MaxSize":
			if value.Kind() == reflect.Int64 {
				xmlConfig.MaxSize = value.Int()
			}
		case "ParseTimeout":
			// Use safe type assertion to avoid panic.
			if dur, ok := value.Interface().(time.Duration); ok {
				xmlConfig.ParseTimeout = dur
			}
		case "CustomTags":
			switch {
			case value.Kind() == reflect.Map:
				// Use safe type assertion for map.
				if tags, ok := value.Interface().(map[string]string); ok {
					xmlConfig.CustomTags = tags
				} else if !value.IsNil() {
					// Try to convert if it's a different map type.
					tags := make(map[string]string)
					for _, key := range value.MapKeys() {
						val := value.MapIndex(key)
						if val.Kind() == reflect.String {
							tags[key.String()] = val.String()
						}
					}
					xmlConfig.CustomTags = tags
				} else {
					xmlConfig.CustomTags = make(map[string]string)
				}
			case !value.IsNil():
				// Try to convert if it's a different map type.
				tags := make(map[string]string)
				for _, key := range value.MapKeys() {
					val := value.MapIndex(key)
					if val.Kind() == reflect.String {
						tags[key.String()] = val.String()
					}
				}
				xmlConfig.CustomTags = tags
			default:
				xmlConfig.CustomTags = make(map[string]string)
			}
		case "IncludeTypeHints":
			if value.Kind() == reflect.Bool {
				xmlConfig.IncludeTypeHints = value.Bool()
			}
		case "PreserveWhitespace":
			if value.Kind() == reflect.Bool {
				xmlConfig.PreserveWhitespace = value.Bool()
			}
		case "ExtraArrayFieldsAsNewlineString":
			if value.Kind() == reflect.Slice && !value.IsNil() {
				out := make([]string, 0, value.Len())
				for i := 0; i < value.Len(); i++ {
					el := value.Index(i)
					if el.Kind() == reflect.String {
						out = append(out, el.String())
					}
				}
				xmlConfig.ExtraArrayFieldsAsNewlineString = out
			}
		case "Logger":
			// Extract logger if it implements stroplog.Logger.
			if value.IsValid() && !value.IsNil() {
				if logger, ok := value.Interface().(stroplog.Logger); ok && logger != nil {
					xmlConfig.Logger = logger
				}
			}
		}
	}

	// In practice, structured_output.Config should always have these fields.

	return xmlConfig
}

// GenerateInstructions creates XML formatting instructions for the LLM.
// It emits a fill-in skeleton derived from the signature: the model copies the
// exact tags and replaces only {{PLACEHOLDER}} tokens (uppercase) with field values.
func (p *XMLParser) GenerateInstructions(signature core.Signature, config interface{}) (string, error) {
	xmlConfig := convertToXMLConfig(config)
	var sb strings.Builder
	var fieldGuide strings.Builder
	var replaceLines []string

	sb.WriteString("Copy this exact XML document. Replace each {{PLACEHOLDER}} listed under \"Replace exactly\".\n")
	sb.WriteString("Placeholders are UPPERCASE inside {{ }} and sit inside a CDATA block between the opening and closing tags.\n")
	sb.WriteString("Keep every <![CDATA[ and ]]> marker exactly as shown. Put the field value only between those markers.\n")
	sb.WriteString("Tag names stay lowercase and must not change. Never put a value in a tag name.\n")
	sb.WriteString("Do not rename, reorder, or omit tags. Do not leave any {{PLACEHOLDER}} unreplaced.\n")
	sb.WriteString("Do not add text outside <response>. Output raw XML only (no markdown code fences).\n\n")

	sb.WriteString("<response>\n")
	fieldNames := make([]string, 0, len(signature.Outputs))
	seenNames := make(map[string]struct{}, len(signature.Outputs))
	for _, output := range signature.Outputs {
		tagName := xmlConfig.GetTagName(output.Name)
		if _, dup := seenNames[tagName]; dup {
			continue
		}
		seenNames[tagName] = struct{}{}
		fieldNames = append(fieldNames, tagName)

		description := strings.TrimSpace(output.Description)
		if description == "" {
			description = output.Name
		}
		if xmlConfig.IncludeTypeHints {
			description = description + " (" + getTypeHint(output.Type) + ")"
		}

		switch {
		case isMapField(output):
			mapKeys := extractExactMapKeys(output.Description, signature.Instruction)
			sb.WriteString(fmt.Sprintf("  <%s>\n", tagName))
			if len(mapKeys) > 0 {
				for _, key := range mapKeys {
					ph := fillPlaceholder(key)
					writeCDATAFillTag(&sb, "    ", key, ph)
					replaceLines = append(replaceLines, fmt.Sprintf("- %s → numeric score for criterion %s only (inside CDATA; never put the score in a tag name)", ph, key))
				}
				fieldGuide.WriteString(fmt.Sprintf("- %s (map): %s. Use the child tags shown; replace each uppercase {{PLACEHOLDER}} inside that child's CDATA with the numeric score.\n", tagName, description))
			} else {
				// No inventable KEY tag — empty parent plus explicit emit rules.
				sb.WriteString(fmt.Sprintf("  </%s>\n", tagName))
				fieldGuide.WriteString(fmt.Sprintf("- %s (map): %s. Inside <%s>, emit one child per map entry with CDATA around the score. Tag name = key (letters/underscores only). Never use a number as a tag name.\n", tagName, description, tagName))
				replaceLines = append(replaceLines, fmt.Sprintf("- <%s> children → one <exact_key><![CDATA[score]]></exact_key> per key from the prompt's criterion ID mapping", tagName))
				continue
			}
			sb.WriteString(fmt.Sprintf("  </%s>\n", tagName))
		case isArrayField(output, xmlConfig):
			childTag := arrayChildTag(tagName)
			ph := fillPlaceholder(childTag)
			sb.WriteString(fmt.Sprintf("  <%s>\n", tagName))
			writeCDATAFillTag(&sb, "    ", childTag, ph)
			sb.WriteString(fmt.Sprintf("  </%s>\n", tagName))
			fieldGuide.WriteString(fmt.Sprintf("- %s (list): %s. Repeat the <%s> CDATA block once per item.\n", tagName, description, childTag))
			replaceLines = append(replaceLines, fmt.Sprintf("- %s → one list item text inside CDATA (repeat the <%s> block for each item)", ph, childTag))
		default:
			ph := fillPlaceholder(tagName)
			writeCDATAFillTag(&sb, "  ", tagName, ph)
			fieldGuide.WriteString(fmt.Sprintf("- %s: %s\n", tagName, description))
			replaceLines = append(replaceLines, fmt.Sprintf("- %s → value for <%s> only (inside that field's CDATA block)", ph, tagName))
		}
	}
	sb.WriteString("</response>\n\n")

	if len(replaceLines) > 0 {
		sb.WriteString("Replace exactly:\n")
		for _, line := range replaceLines {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if fieldGuide.Len() > 0 {
		sb.WriteString("Field meanings (do not paste these lines into XML values):\n")
		sb.WriteString(fieldGuide.String())
		sb.WriteString("\n")
	}

	sb.WriteString("Rules:\n")
	sb.WriteString(fmt.Sprintf("1. Use exactly these %d field(s) in order: %s\n", len(fieldNames), strings.Join(fieldNames, ", ")))
	sb.WriteString("2. Close each tag before opening the next one\n")
	sb.WriteString("3. The last tag must be </response>\n")
	sb.WriteString("4. Replace every uppercase {{PLACEHOLDER}}; never leave braces in the output\n")
	sb.WriteString("5. CDATA wraps every leaf value (prose, list items, and numeric scores) so template tags stay separate from data\n")
	sb.WriteString("6. Keep every <![CDATA[ and ]]> marker; do not write ]]> inside field content\n")
	sb.WriteString("7. Never use a number (e.g. 2.0) or score as an XML tag name\n")
	sb.WriteString("8. Angle brackets inside CDATA are fine (e.g. <2.0); do not put them outside CDATA\n")
	sb.WriteString("9. No mixed encodings: do not wrap JSON inside XML field text unless that field's description explicitly requires JSON; prefer delimiter lines inside <item> over stringified objects\n")
	sb.WriteString("10. Do not echo this template (or any <response>...</response> example) inside a field value\n")

	if xmlConfig.StrictParsing {
		sb.WriteString("11. Include ALL required fields with non-empty values\n")
	}
	if !xmlConfig.PreserveWhitespace {
		sb.WriteString("12. Avoid unnecessary whitespace outside CDATA blocks\n")
	}

	return sb.String(), nil
}

// fillPlaceholder returns an uppercase fill-in token, e.g. score → {{SCORE}}.
func fillPlaceholder(name string) string {
	return "{{" + strings.ToUpper(name) + "}}"
}

// plainTextFieldNames returns first-level output fields that are neither maps nor arrays.
// Their bodies are sanitized for bare "<" before XML parsing.
func plainTextFieldNames(signature core.Signature, config XMLConfig) []string {
	names := make([]string, 0, len(signature.Outputs))
	seen := make(map[string]struct{}, len(signature.Outputs))
	for _, output := range signature.Outputs {
		name := strings.TrimSpace(output.Name)
		if name == "" {
			continue
		}
		if isMapField(output) || isArrayField(output, config) {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

// xmlTagTokenRe matches opening, closing, and self-closing element tags (not CDATA/declarations).
var xmlTagTokenRe = regexp.MustCompile(`</?([A-Za-z_][\w.-]*)(?:\s[^>]*)?\s*/?>`)

// repairUnclosedElementTags closes still-open tags when the model truncated output before </response>.
// It is a no-op when </response> is already present so well-formed documents are unchanged.
func repairUnclosedElementTags(xmlContent string) string {
	if xmlContent == "" {
		return xmlContent
	}
	lower := strings.ToLower(xmlContent)
	if strings.Contains(lower, "</response>") {
		return xmlContent
	}
	content := stripIncompleteTrailingTag(xmlContent)
	stack := openTagStack(content)
	if len(stack) == 0 {
		return content
	}
	var b strings.Builder
	b.WriteString(content)
	for i := len(stack) - 1; i >= 0; i-- {
		b.WriteString("</")
		b.WriteString(stack[i])
		b.WriteString(">")
	}
	return b.String()
}

func stripIncompleteTrailingTag(s string) string {
	s = strings.TrimRight(s, " \t\n\r")
	last := strings.LastIndex(s, "<")
	if last < 0 {
		return s
	}
	if strings.Index(s[last:], ">") < 0 {
		return strings.TrimRight(s[:last], " \t\n\r")
	}
	return s
}

func openTagStack(xmlContent string) []string {
	var stack []string
	for _, m := range xmlTagTokenRe.FindAllStringSubmatch(xmlContent, -1) {
		full := m[0]
		name := m[1]
		if strings.HasPrefix(full, "</") {
			for len(stack) > 0 && stack[len(stack)-1] != name {
				stack = stack[:len(stack)-1]
			}
			if len(stack) > 0 && stack[len(stack)-1] == name {
				stack = stack[:len(stack)-1]
			}
			continue
		}
		if isSelfClosingXMLTag(full) {
			continue
		}
		stack = append(stack, name)
	}
	return stack
}

func isSelfClosingXMLTag(tag string) bool {
	t := strings.TrimSpace(tag)
	t = strings.TrimSuffix(t, ">")
	return strings.HasSuffix(t, "/")
}

// repairUnclosedCDATA inserts ]]> before the next </tag> (or at EOF) when the model opened
// <![CDATA[ but omitted the closer. This is common on numeric score leaves.
func repairUnclosedCDATA(xmlContent string) string {
	const open = "<![CDATA["
	const close = "]]>"
	if xmlContent == "" || !strings.Contains(xmlContent, open) {
		return xmlContent
	}
	var b strings.Builder
	remaining := xmlContent
	for {
		i := strings.Index(remaining, open)
		if i < 0 {
			b.WriteString(remaining)
			break
		}
		b.WriteString(remaining[:i+len(open)])
		remaining = remaining[i+len(open):]
		j := strings.Index(remaining, close)
		k := strings.Index(remaining, "</")
		if j >= 0 && (k < 0 || j < k) {
			b.WriteString(remaining[:j+len(close)])
			remaining = remaining[j+len(close):]
			continue
		}
		if k >= 0 {
			b.WriteString(remaining[:k])
			b.WriteString(close)
			remaining = remaining[k:]
			continue
		}
		b.WriteString(remaining)
		b.WriteString(close)
		remaining = ""
	}
	return b.String()
}

// sanitizePlainTextFieldAngleBrackets escapes bare "<" inside plain-text field bodies so
// comparisons like "score < 2.0" do not become invalid tags. Well-formed CDATA bodies are left unchanged.
func sanitizePlainTextFieldAngleBrackets(xmlContent string, plainFields []string) string {
	if xmlContent == "" || len(plainFields) == 0 {
		return xmlContent
	}
	result := xmlContent
	for _, field := range plainFields {
		result = sanitizeOnePlainTextField(result, field)
	}
	return result
}

func sanitizeOnePlainTextField(xmlContent, field string) string {
	open := "<" + field + ">"
	closeTag := "</" + field + ">"
	var b strings.Builder
	remaining := xmlContent
	for {
		start := strings.Index(remaining, open)
		if start < 0 {
			b.WriteString(remaining)
			break
		}
		b.WriteString(remaining[:start+len(open)])
		remaining = remaining[start+len(open):]
		end := strings.Index(remaining, closeTag)
		if end < 0 {
			b.WriteString(remaining)
			remaining = ""
			break
		}
		body := remaining[:end]
		remaining = remaining[end:] // begins with closeTag
		b.WriteString(sanitizePlainTextFieldBody(body))
	}
	return b.String()
}

func sanitizePlainTextFieldBody(body string) string {
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "<![CDATA[") && strings.HasSuffix(trimmed, "]]>") {
		return body
	}
	var out strings.Builder
	for i := 0; i < len(body); {
		if body[i] != '<' {
			out.WriteByte(body[i])
			i++
			continue
		}
		if strings.HasPrefix(body[i:], "<![CDATA[") {
			end := strings.Index(body[i:], "]]>")
			if end >= 0 {
				out.WriteString(body[i : i+end+3])
				i += end + 3
				continue
			}
		}
		out.WriteString("&lt;")
		i++
	}
	return out.String()
}

// writeCDATAFillTag writes open tag, CDATA-wrapped placeholder, then close tag.
// Default for all leaf values so template tags stay separate from field data.
func writeCDATAFillTag(sb *strings.Builder, indent, tagName, placeholder string) {
	sb.WriteString(indent)
	sb.WriteString("<")
	sb.WriteString(tagName)
	sb.WriteString(">\n")
	sb.WriteString(indent)
	sb.WriteString("<![CDATA[\n")
	sb.WriteString(indent)
	sb.WriteString(placeholder)
	sb.WriteString("\n")
	sb.WriteString(indent)
	sb.WriteString("]]>\n")
	sb.WriteString(indent)
	sb.WriteString("</")
	sb.WriteString(tagName)
	sb.WriteString(">\n")
}

// arrayChildTag chooses the repeated child element name for list fields.
func arrayChildTag(parentTag string) string {
	if strings.Contains(strings.ToLower(parentTag), "question") {
		return "question"
	}
	return "item"
}

// exactMapKeysMarker prefixes an explicit comma-separated key list in a field description.
const exactMapKeysMarker = "Exact map keys:"

// criterionIDMappingArrow matches score-prompt lines like: - Name → "instruction_compliance"
var criterionIDMappingArrow = regexp.MustCompile(`→\s*"([a-z][a-z0-9_]*)"`)

// extractExactMapKeys finds fixed map child tag names from the field description and/or
// signature instruction (criterion ID mapping). Returns nil when keys are unknown.
func extractExactMapKeys(description, instruction string) []string {
	keys := parseExactMapKeysList(description)
	if len(keys) == 0 {
		keys = parseCriterionIDsFromInstruction(instruction)
	}
	return keys
}

func parseExactMapKeysList(description string) []string {
	idx := strings.Index(description, exactMapKeysMarker)
	if idx < 0 {
		return nil
	}
	rest := strings.TrimSpace(description[idx+len(exactMapKeysMarker):])
	if end := strings.IndexAny(rest, ".\n"); end >= 0 {
		rest = rest[:end]
	}
	return normalizeMapKeys(strings.Split(rest, ","))
}

func parseCriterionIDsFromInstruction(instruction string) []string {
	if instruction == "" {
		return nil
	}
	matches := criterionIDMappingArrow.FindAllStringSubmatch(instruction, -1)
	if len(matches) == 0 {
		return nil
	}
	raw := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			raw = append(raw, m[1])
		}
	}
	return normalizeMapKeys(raw)
}

func normalizeMapKeys(parts []string) []string {
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		key := strings.TrimSpace(part)
		key = strings.Trim(key, `"'`)
		if key == "" || !isValidXMLName(key) {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func isValidXMLName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case i == 0 && ((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'):
			continue
		case i > 0 && ((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'):
			continue
		default:
			return false
		}
	}
	return true
}

// InjectInstructions adds XML formatting instructions to the input.
func (p *XMLParser) InjectInstructions(inputs map[string]any, instructions string, signature core.Signature) error {
	// Find the best input field to inject instructions into.
	targetField := findInstructionTargetField(signature)
	if targetField == "" {
		return fmt.Errorf("no suitable input field found for XML instructions")
	}

	// Get current value and append instructions.
	currentValue, exists := inputs[targetField]
	if !exists {
		inputs[targetField] = instructions
		return nil
	}

	// Convert to string and append.
	currentStr := fmt.Sprintf("%v", currentValue)
	inputs[targetField] = currentStr + "\n\n" + instructions

	return nil
}

// FindResponseText locates the text content to parse from outputs.
func (p *XMLParser) FindResponseText(outputs map[string]any) string {
	// Raw response key differs across dspy-go / provider paths (__raw_response vs _raw_response).
	if text, _ := rawresponse.TextFrom(outputs); text != "" {
		return text
	}

	// Priority order for finding response text.
	candidates := []string{"response", "output", "result", "answer", "text"}

	for _, candidate := range candidates {
		if text, exists := outputs[candidate]; exists {
			if textStr, ok := text.(string); ok && textStr != "" {
				return textStr
			}
		}
	}

	// Fallback: look for any string value that contains XML content.
	for _, value := range outputs {
		if textStr, ok := value.(string); ok && textStr != "" {
			// Check if this string contains XML-like content.
			if strings.Contains(textStr, "<") && strings.Contains(textStr, ">") {
				return textStr
			}
		}
	}

	// Final fallback: use first non-empty string value.
	for _, value := range outputs {
		if textStr, ok := value.(string); ok && textStr != "" {
			return textStr
		}
	}

	return ""
}

// ExtractContent extracts XML from potentially mixed content.
// It prefers a </response>- or </stories>-bounded slice so ">" inside element text does not truncate.
func (p *XMLParser) ExtractContent(text string) string {
	return extractXMLDocumentSlice(text)
}

// extractXMLDocumentSlice returns the XML document for parsing. It prefers a </response>-bounded
// slice so a literal ">" inside field text (e.g. "3 > 2" in a summary) does not truncate the
// document when using first-< to last-> heuristics. If </response> is missing (truncated stream),
// returns from <response through end of text.
func extractXMLDocumentSlice(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		startIdx := strings.Index(text, "\n")
		if startIdx != -1 {
			endIdx := strings.LastIndex(text, "```")
			if endIdx > startIdx {
				text = strings.TrimSpace(text[startIdx+1 : endIdx])
			}
		}
	}
	lower := strings.ToLower(text)
	const closeResponse = "</response>"
	if idx := strings.Index(lower, "<response"); idx >= 0 {
		if j := strings.LastIndex(lower, closeResponse); j >= idx {
			return strings.TrimSpace(text[idx : j+len(closeResponse)])
		}
		return strings.TrimSpace(text[idx:])
	}
	const closeStories = "</stories>"
	if idx := strings.Index(lower, "<stories"); idx >= 0 {
		if j := strings.LastIndex(lower, closeStories); j >= idx {
			return strings.TrimSpace(text[idx : j+len(closeStories)])
		}
		return strings.TrimSpace(text[idx:])
	}
	return extractContentLegacy(text)
}

// extractContentLegacy is first-< to last-> extraction when no response/stories wrapper is found.
func extractContentLegacy(text string) string {
	text = strings.TrimSpace(text)
	start := strings.Index(text, "<")
	if start == -1 {
		return ""
	}
	end := strings.LastIndex(text, ">")
	if end == -1 || end <= start {
		return ""
	}
	return strings.TrimSpace(text[start : end+1])
}

// ParseOutputs extracts structured data from XML-formatted outputs.
func (p *XMLParser) ParseOutputs(ctx context.Context, outputs map[string]any, signature core.Signature, config interface{}) (map[string]any, error) {
	xmlConfig := convertToXMLConfig(config)

	// Debug: Log if logger is available (helps diagnose reflection extraction issues).
	if xmlConfig.Logger != nil {
		xmlConfig.Logger.Debug("XML parser: Logger is available for debug logging")
	}

	// Find the response field (usually the main output).
	responseText := p.FindResponseText(outputs)
	if responseText == "" {
		// Note: This is NOT an error - it just means there's nothing to parse.
		if xmlConfig.Logger != nil {
			xmlConfig.Logger.WithFields(map[string]interface{}{
				"output_keys": getOutputKeys(outputs),
			}).Debug("XML parser: no response text found in outputs")
		}
		return outputs, nil
	}

	// Log response text preview for debugging.
	if xmlConfig.Logger != nil {
		preview := responseText
		if len(preview) > 500 {
			preview = preview[:500] + "... (truncated)"
		}
		xmlConfig.Logger.WithFields(map[string]interface{}{
			"response_length":  len(responseText),
			"response_preview": preview,
		}).Debug("XML parser: found response text")
	}

	// Check size limits.
	if len(responseText) > int(xmlConfig.MaxSize) {
		err := fmt.Errorf("XML response size (%d bytes) exceeds limit (%d bytes)",
			len(responseText), xmlConfig.MaxSize)
		if xmlConfig.Logger != nil {
			xmlConfig.Logger.WithError(err).Error("XML parser: response size exceeds limit")
		}
		return nil, err
	}

	// Parse with timeout.
	ctx, cancel := context.WithTimeout(ctx, xmlConfig.ParseTimeout)
	defer cancel()

	parsedFields, err := p.parseXMLWithTimeout(ctx, responseText, signature, xmlConfig)
	if err != nil {
		if xmlConfig.Logger != nil {
			// Log the full response text for debugging (truncated if too long).
			fullResponse := responseText
			if len(fullResponse) > 2000 {
				fullResponse = fullResponse[:2000] + "... (truncated, total length: " + strconv.Itoa(len(responseText)) + ")"
			}
			xmlConfig.Logger.WithFields(map[string]interface{}{
				"error":            err.Error(),
				"fallback_enabled": xmlConfig.FallbackToText,
				"response_text":    fullResponse,
				"response_length":  len(responseText),
			}).Warn("XML parser: parsing failed")
		}
		if xmlConfig.FallbackToText {
			// Return original outputs if parsing fails and fallback is enabled.
			// Still filter to only signature fields to avoid leaking raw XML containers.
			if xmlConfig.Logger != nil {
				xmlConfig.Logger.Debug("XML parser: falling back to raw outputs")
			}

			// Build a whitelist of allowed field names from the signature.
			allowedFields := make(map[string]bool)
			for _, output := range signature.Outputs {
				allowedFields[output.Name] = true
			}

			// Only return fields that are in the signature.
			result := make(map[string]any)
			for k, v := range outputs {
				// Skip internal fields.
				if strings.HasPrefix(k, "_") {
					continue
				}
				// Only include fields in signature.
				if allowedFields[k] {
					result[k] = v
				}
			}

			return result, nil
		}
		return nil, fmt.Errorf("XML parsing failed: %w", err)
	}

	// Log parsed fields for debugging.
	if xmlConfig.Logger != nil {
		parsedKeys := make([]string, 0, len(parsedFields))
		for k := range parsedFields {
			parsedKeys = append(parsedKeys, k)
		}
		xmlConfig.Logger.WithFields(map[string]interface{}{
			"parsed_keys":  parsedKeys,
			"parsed_count": len(parsedFields),
		}).Debug("XML parser: parsing succeeded")
	}

	// After successful parsing, only return fields that are in the module signature.
	// This ensures we only return what the module is supposed to produce.
	// Raw XML container fields (response, output, result, answer, text) are automatically
	// excluded since they're not in the signature.
	result := make(map[string]any)

	// Build a whitelist of allowed field names from the signature.
	allowedFields := make(map[string]bool)
	for _, output := range signature.Outputs {
		allowedFields[output.Name] = true
	}

	// Only include parsed fields that are in the signature.
	for k, v := range parsedFields {
		if allowedFields[k] {
			result[k] = v
		} else if xmlConfig.Logger != nil {
			// Log fields that were parsed but not in signature (helps catch issues).
			xmlConfig.Logger.WithFields(map[string]interface{}{
				"field_name": k,
			}).Debug("XML parser: skipping field not in signature")
		}
	}

	return result, nil
}

// getOutputKeys extracts keys from outputs map for logging.
func getOutputKeys(outputs map[string]any) []string {
	keys := make([]string, 0, len(outputs))
	for k := range outputs {
		keys = append(keys, k)
	}
	return keys
}

// parseXMLWithTimeout parses XML with context timeout support.
func (p *XMLParser) parseXMLWithTimeout(ctx context.Context, responseText string, signature core.Signature, config XMLConfig) (map[string]any, error) {
	// Channel for result communication.
	resultChan := make(chan parseResult, 1)

	go func() {
		result, err := p.parseXML(responseText, signature, config)
		resultChan <- parseResult{result: result, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("XML parsing timeout: %w", ctx.Err())
	case res := <-resultChan:
		return res.result, res.err
	}
}

type parseResult struct {
	result map[string]any
	err    error
}

// parseXML performs the actual XML parsing with support for nested elements.
func (p *XMLParser) parseXML(responseText string, signature core.Signature, config XMLConfig) (map[string]any, error) {
	// Get or create cached signature info.
	sigInfo := p.getSignatureInfo(signature, config)

	// Pre-validate XML if enabled.
	if config.ValidateXML {
		if err := p.validateXMLSyntax(responseText); err != nil {
			return nil, fmt.Errorf("XML validation failed: %w", err)
		}
	}

	// Extract XML content (handle cases where XML is embedded in text).
	xmlContent := p.ExtractContent(responseText)
	if xmlContent == "" {
		// Try to provide more helpful error message.
		preview := responseText
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return nil, fmt.Errorf("no XML content found in response (length: %d). Preview: %s", len(responseText), preview)
	}

	// Pre-process XML to escape common unescaped entities from LLM output.
	xmlContent = p.escapeXMLEntities(xmlContent)
	// Close CDATA sections the model opened but forgot to terminate before </tag>.
	xmlContent = repairUnclosedCDATA(xmlContent)
	if config.StrictParsing && !strings.Contains(strings.ToLower(xmlContent), "</response>") {
		return nil, fmt.Errorf("XML truncated: missing </response>")
	}
	// Close still-open tags when the stream ended before </response> (truncated LLM output).
	xmlContent = repairUnclosedElementTags(xmlContent)
	// Escape bare "<" inside plain-text field bodies (e.g. "score < 2.0" in directives_ack)
	// so comparisons are not parsed as tags when CDATA was omitted. Map/array structure is left alone.
	xmlContent = sanitizePlainTextFieldAngleBrackets(xmlContent, plainTextFieldNames(signature, config))

	// Parse XML using Go's encoding/xml.
	decoder := xml.NewDecoder(strings.NewReader(xmlContent))
	decoder.CharsetReader = p.charsetReader

	fields := make(map[string]any)
	depth := 0
	var currentTag string
	var currentMapField string       // Track if we're inside a map field.
	var mapBuilder map[string]string // Builder for current map field.
	var openedFirstLevelTag string   // Track the currently opened first-level tag name.
	var currentArrayField string     // Track if we're inside an array field (e.g. sub_questions).
	var arrayBuilder []string        // Collect items for current array field.
	var currentArrayItem strings.Builder
	var mapLooseText strings.Builder // Direct text under a map parent (model may emit JSON object).
	// Newline-joined array fields (see NewlineJoinedArrayFields) collect repeated XML children but
	// flush to one string; direct text under the parent tag (no wrapper) is accumulated here when no items exist.
	var arrayJoinedLooseText strings.Builder

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("XML parsing error: %w", err)
		}

		switch t := token.(type) {
		case xml.StartElement:
			depth++
			if depth > config.MaxDepth {
				return nil, fmt.Errorf("XML depth limit exceeded: %d", depth)
			}

			tagName := t.Name.Local

			switch depth {
			case 1:
				// Skip root tag (typically <response>).
				currentTag = ""
				currentMapField = ""
				mapBuilder = nil
				mapLooseText.Reset()
				currentArrayField = ""
				arrayBuilder = nil
			case 2:
				// First-level field.
				currentTag = tagName
				openedFirstLevelTag = tagName // Track which first-level tag we opened.
				currentMapField = ""
				mapBuilder = nil
				mapLooseText.Reset()
				currentArrayField = ""
				arrayBuilder = nil

				// Check if this is an array field (list of items with repeated child elements).
				if fieldName, exists := sigInfo.TagMap[tagName]; exists {
					if sigInfo.ArrayFields[fieldName] {
						currentArrayField = fieldName
						arrayBuilder = make([]string, 0)
						currentTag = "" // Nested elements will be collected as items.
						if sigInfo.NewlineJoinedArrayFields[fieldName] {
							arrayJoinedLooseText.Reset()
						}
					} else if sigInfo.MapFields[fieldName] {
						// This is a map field - start collecting nested elements.
						currentMapField = fieldName
						mapBuilder = make(map[string]string)
						mapLooseText.Reset()
						// Only nested elements (depth > 2) should contribute to the map.
						currentTag = ""
					}
				}
			default:
				// Depth > 2: nested element.
				switch {
				case currentArrayField != "":
					// Inside array field: we'll collect this element's text as one item.
					currentTag = tagName
					currentArrayItem.Reset()
				case currentMapField != "" && mapBuilder != nil:
					// Store the tag name as the key, we'll get the value from CharData.
					currentTag = tagName
				default:
					// Not in a map or array field - ignore nested elements (dspy-go behavior).
					currentTag = ""
				}
			}

		case xml.EndElement:
			depth--

			switch depth {
			case 0:
				// Finished root tag.
				currentTag = ""
				currentMapField = ""
				mapBuilder = nil
				mapLooseText.Reset()
				currentArrayField = ""
				arrayBuilder = nil
				arrayJoinedLooseText.Reset()
				openedFirstLevelTag = ""
			case 1:
				// Finished a first-level field.
				switch {
				case currentArrayField != "" && openedFirstLevelTag != "":
					// Flush array field: store collected items as []interface{}, except newline-joined Text fields.
					fieldName := currentArrayField
					if sigInfo.NewlineJoinedArrayFields[fieldName] {
						if len(arrayBuilder) > 0 {
							parts := make([]string, 0, len(arrayBuilder))
							for _, s := range arrayBuilder {
								s = strings.TrimSpace(s)
								if s != "" {
									parts = append(parts, s)
								}
							}
							fields[fieldName] = strings.Join(parts, "\n")
						} else {
							fields[fieldName] = strings.TrimSpace(arrayJoinedLooseText.String())
						}
					} else {
						sliceIf := make([]interface{}, len(arrayBuilder))
						for i, s := range arrayBuilder {
							sliceIf[i] = s
						}
						fields[fieldName] = sliceIf
					}
				case currentMapField != "" && mapBuilder != nil && len(mapBuilder) > 0:
					// Convert map builder to map[string]interface{} with proper types.
					fieldName := currentMapField
					if field, exists := sigInfo.FieldMap[fieldName]; exists {
						typedMap, err := p.convertMapField(mapBuilder, field)
						if err != nil {
							return nil, fmt.Errorf("failed to convert map field %s: %w", fieldName, err)
						}
						fields[fieldName] = typedMap
					} else {
						fields[fieldName] = mapStringStringToInterface(mapBuilder)
					}
				case currentMapField != "" && mapBuilder != nil && len(mapBuilder) == 0:
					fieldName := currentMapField
					if loose := strings.TrimSpace(mapLooseText.String()); loose != "" {
						typedMap, err := parseLooseMapFieldText(loose, fieldName)
						switch {
						case err == nil:
							fields[fieldName] = typedMap
						case strings.HasPrefix(loose, "{"):
							return nil, fmt.Errorf("failed to parse map field %s: %w", fieldName, err)
						default:
							// Scalar or prose under the map tag (e.g. CDATA "2.0") — leave empty map for downstream validation.
							fields[fieldName] = make(map[string]interface{})
						}
					} else if fieldName, exists := sigInfo.TagMap[openedFirstLevelTag]; exists {
						if _, alreadySet := fields[fieldName]; !alreadySet {
							fields[fieldName] = make(map[string]interface{})
						}
					}
				case openedFirstLevelTag != "":
					// Handle empty tags: if a first-level field was opened but never populated,
					// set it to empty string (for string fields), empty map (for map fields), or empty slice (for array fields).
					if fieldName, exists := sigInfo.TagMap[openedFirstLevelTag]; exists {
						// Check if this field was already populated (e.g., by CharData or array flush).
						if _, alreadySet := fields[fieldName]; !alreadySet {
							switch {
							case currentArrayField != "":
								// It's an array field (possibly empty).
								if sigInfo.NewlineJoinedArrayFields[fieldName] {
									fields[fieldName] = ""
								} else {
									fields[fieldName] = []interface{}{}
								}
							case currentMapField != "":
								// It's a map field that was empty - set to empty map.
								fields[fieldName] = make(map[string]interface{})
							default:
								// It's a regular field that was empty - set to empty string.
								fields[fieldName] = ""
							}
						}
					}
				}
				currentTag = ""
				currentMapField = ""
				mapBuilder = nil
				mapLooseText.Reset()
				currentArrayField = ""
				arrayBuilder = nil
				// arrayJoinedLooseText: reset when a newline-joined array field opens (StartElement depth 2)
				// or at end of document (EndElement depth 0). Do not reset on every first-level close:
				// loose text after nested tags still belongs to the same field until its parent closes.
				openedFirstLevelTag = ""
			case 2:
				// Finished a nested element (e.g. one <question> inside <sub_questions>).
				if currentArrayField != "" && currentArrayItem.Len() > 0 {
					item := strings.TrimSpace(currentArrayItem.String())
					if item != "" {
						arrayBuilder = append(arrayBuilder, item)
					}
					currentArrayItem.Reset()
				}
				currentTag = ""
			default:
				// Depth > 1, continue processing nested elements
			}

		case xml.CharData:
			// Direct text under a newline-joined array parent (no child wrapper): capture plain lines when
			// the model omits repeated item tags; array mode normally ignores CharData until a child opens.
			if currentArrayField != "" && sigInfo.NewlineJoinedArrayFields[currentArrayField] && currentTag == "" {
				arrayJoinedLooseText.WriteString(string(t))
			} else if currentMapField != "" && currentTag == "" {
				mapLooseText.WriteString(string(t))
			} else if currentTag != "" {
				content := string(t)
				if !config.PreserveWhitespace {
					content = strings.TrimSpace(content)
				}

				if currentArrayField != "" {
					// Inside array field: append to current item (e.g. text inside <question>).
					currentArrayItem.WriteString(content)
				} else if currentMapField != "" && mapBuilder != nil {
					// Strip field name prefix if present.
					content = p.stripFieldPrefix(content, currentTag)
					// Concatenate if multiple CharData tokens exist for the same key.
					if existing, exists := mapBuilder[currentTag]; exists {
						mapBuilder[currentTag] = existing + content
					} else {
						mapBuilder[currentTag] = content
					}
				} else if fieldName, exists := sigInfo.TagMap[currentTag]; exists {
					// Strip field name prefix if present.
					content = p.stripFieldPrefix(content, fieldName)
					// Skip empty CharData (e.g. newlines around a CDATA block) so they do not
					// overwrite real content already captured for this field.
					if content == "" {
						break
					}

					// Type conversion based on field type.
					if field, exists := sigInfo.FieldMap[fieldName]; exists {
						typedValue, err := p.convertFieldValue(content, field)
						if err != nil {
							return nil, fmt.Errorf("field %s conversion failed: %w", fieldName, err)
						}
						if existing, ok := fields[fieldName].(string); ok && existing != "" {
							if typedStr, ok := typedValue.(string); ok {
								fields[fieldName] = existing + typedStr
								break
							}
						}
						fields[fieldName] = typedValue
					}
				}
			}
		}
	}

	// Validate required fields if strict parsing is enabled.
	if config.StrictParsing {
		if err := p.validateRequiredFields(fields, sigInfo); err != nil {
			return nil, err
		}
	}

	return fields, nil
}

// getSignatureInfo retrieves or creates cached signature parsing information.
// Uses double-checked locking with LRU eviction to prevent unbounded cache growth.
func (p *XMLParser) getSignatureInfo(signature core.Signature, config XMLConfig) *ParsedSignature {
	key := p.signatureKey(signature, config)

	// First check with read lock.
	p.mutex.RLock()
	if cached, exists := p.cache[key]; exists {
		p.mutex.RUnlock()
		return cached
	}
	p.mutex.RUnlock()

	// Create new signature info.
	sigInfo := &ParsedSignature{
		FieldMap:                 make(map[string]core.OutputField),
		TagMap:                   make(map[string]string),
		RequiredSet:              make(map[string]bool),
		MapFields:                make(map[string]bool),
		ArrayFields:              make(map[string]bool),
		NewlineJoinedArrayFields: make(map[string]bool),
	}

	for _, output := range signature.Outputs {
		sigInfo.FieldMap[output.Name] = output
		tagName := config.GetTagName(output.Name)
		sigInfo.TagMap[tagName] = output.Name
		sigInfo.RequiredSet[output.Name] = true // Consider all outputs required for now.

		// Detect map fields based on description or field name patterns.
		if isMapField(output) {
			sigInfo.MapFields[output.Name] = true
		}
		// Detect array fields (list of items); do not treat as map.
		if isArrayField(output, config) {
			sigInfo.ArrayFields[output.Name] = true
		}
	}
	joined := newlineJoinedArrayFieldSet(config)
	for _, output := range signature.Outputs {
		if output.Type != core.FieldTypeText {
			continue
		}
		if _, ok := joined[output.Name]; !ok {
			continue
		}
		if sigInfo.ArrayFields[output.Name] {
			sigInfo.NewlineJoinedArrayFields[output.Name] = true
		}
	}

	// Double-checked locking: acquire write lock and check again.
	p.mutex.Lock()
	// Re-check after acquiring write lock to avoid race condition.
	if cached, exists := p.cache[key]; exists {
		p.mutex.Unlock()
		return cached
	}

	// Evict oldest entry if cache is full (LRU eviction).
	if len(p.cache) >= maxCacheSize {
		// Remove oldest entry.
		oldestKey := p.cacheOrder[0]
		delete(p.cache, oldestKey)
		// Remove from order list.
		p.cacheOrder = p.cacheOrder[1:]
	}

	// Add new entry.
	p.cache[key] = sigInfo
	// Add to end of order list (most recently used).
	p.cacheOrder = append(p.cacheOrder, key)
	p.mutex.Unlock()

	return sigInfo
}

// signatureKey generates a cache key for the signature.
func (p *XMLParser) signatureKey(signature core.Signature, config XMLConfig) string {
	var sb strings.Builder
	for _, output := range signature.Outputs {
		sb.WriteString(output.Name)
		sb.WriteString(":")
		sb.WriteString(string(output.Type))
		sb.WriteString(":")
		sb.WriteString(output.Description)
		sb.WriteString(";")
	}
	sb.WriteByte('|')
	keys := make([]string, 0, len(config.CustomTags))
	for k := range config.CustomTags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(config.CustomTags[k])
		sb.WriteByte(';')
	}
	sb.WriteByte('|')
	ex := append([]string(nil), config.ExtraArrayFieldsAsNewlineString...)
	sort.Strings(ex)
	for _, e := range ex {
		sb.WriteString(e)
		sb.WriteByte(';')
	}
	return sb.String()
}

// validateXMLSyntax performs basic XML syntax validation.
func (p *XMLParser) validateXMLSyntax(xmlText string) error {
	decoder := xml.NewDecoder(strings.NewReader(xmlText))
	for {
		_, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// convertFieldValue converts string content to the appropriate type.
func (p *XMLParser) convertFieldValue(content string, field core.OutputField) (any, error) {
	switch field.Type {
	case core.FieldTypeText, "":
		return content, nil
	case core.FieldTypeImage:
		// For image fields, return the content as-is (could be URL or description).
		return content, nil
	case core.FieldTypeAudio:
		// For audio fields, return the content as-is (could be URL or description).
		return content, nil
	default:
		// Try to infer type from content.
		return p.inferTypeFromContent(content)
	}
}

// mapStringStringToInterface converts map builder values to map[string]interface{}.
func mapStringStringToInterface(mapBuilder map[string]string) map[string]interface{} {
	result := make(map[string]interface{}, len(mapBuilder))
	for key, valueStr := range mapBuilder {
		result[key] = valueStr
	}
	return result
}

// parseLooseMapFieldText parses direct text under a map XML field (often a JSON object).
func parseLooseMapFieldText(text, fieldName string) (map[string]interface{}, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("map field %s has empty text content", fieldName)
	}
	if strings.HasPrefix(text, "{") {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(text), &parsed); err != nil {
			return nil, fmt.Errorf("JSON object parse failed: %w", err)
		}
		if len(parsed) == 0 {
			return nil, fmt.Errorf("JSON object is empty")
		}
		return parsed, nil
	}
	return nil, fmt.Errorf("expected nested XML child tags or JSON object, got plain text preview %q", truncateMapTextPreview(text))
}

func truncateMapTextPreview(s string) string {
	if len(s) <= 120 {
		return s
	}
	return s[:120] + "..."
}

// convertMapField converts a map[string]string to map[string]interface{} with proper types.
// This handles map fields like criterion_scores where values should be floats.
func (p *XMLParser) convertMapField(mapBuilder map[string]string, field core.OutputField) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for key, valueStr := range mapBuilder {
		// Try to infer type from content (similar to convertFieldValue).
		typedValue, err := p.inferTypeFromContent(valueStr)
		if err != nil {
			return nil, fmt.Errorf("failed to convert map value for key %s: %w", key, err)
		}
		result[key] = typedValue
	}
	return result, nil
}

// inferTypeFromContent attempts to infer and convert content type.
func (p *XMLParser) inferTypeFromContent(content string) (any, error) {
	content = strings.TrimSpace(content)

	// Try boolean.
	if strings.EqualFold(content, "true") {
		return true, nil
	}
	if strings.EqualFold(content, "false") {
		return false, nil
	}

	// Try integer.
	if intVal, err := strconv.ParseInt(content, 10, 64); err == nil {
		return intVal, nil
	}

	// Try float.
	if floatVal, err := strconv.ParseFloat(content, 64); err == nil {
		return floatVal, nil
	}

	// Default to string.
	return content, nil
}

// validateRequiredFields checks that all required fields are present.
func (p *XMLParser) validateRequiredFields(fields map[string]any, sigInfo *ParsedSignature) error {
	for fieldName := range sigInfo.RequiredSet {
		if _, exists := fields[fieldName]; !exists {
			return fmt.Errorf("required field missing: %s", fieldName)
		}
	}
	return nil
}

// escapeXMLEntities escapes common unescaped entities in LLM-generated XML.
func (p *XMLParser) escapeXMLEntities(xmlContent string) string {
	// First, temporarily mark valid entities to protect them.
	validEntities := []string{"&amp;", "&lt;", "&gt;", "&quot;", "&apos;"}
	placeholders := make(map[string]string)

	// Protect existing valid entities.
	for i, entity := range validEntities {
		placeholder := fmt.Sprintf("__ENTITY_%d__", i)
		placeholders[placeholder] = entity
		xmlContent = strings.ReplaceAll(xmlContent, entity, placeholder)
	}

	// Now escape all remaining & characters.
	xmlContent = strings.ReplaceAll(xmlContent, "&", "&amp;")

	// Restore the protected entities.
	for placeholder, entity := range placeholders {
		xmlContent = strings.ReplaceAll(xmlContent, placeholder, entity)
	}

	return xmlContent
}

// stripFieldPrefix strips field name prefix if present (e.g., "answer: 366" -> "366").
func (p *XMLParser) stripFieldPrefix(content, fieldName string) string {
	// Check if content starts with "fieldname:" pattern (case-insensitive).
	prefix := fieldName + ":"
	if strings.HasPrefix(strings.ToLower(content), strings.ToLower(prefix)) {
		// then trim any whitespace/newlines.
		return strings.TrimLeft(content[len(prefix):], " \n\t")
	}
	return content
}

// charsetReader handles charset encoding for XML decoder.
func (p *XMLParser) charsetReader(charset string, input io.Reader) (io.Reader, error) {
	if charset != "utf-8" && charset != "UTF-8" && charset != "" {
		return nil, fmt.Errorf("unsupported charset: %s", charset)
	}
	return input, nil
}

// Helper functions.

// isMapField determines if a field is expected to be a key-value map (nested XML).
// Only "map" and "dictionary" are used; "object" is not, because in descriptions it usually
// means "entity" (e.g. "chapter object") not "key-value structure".
func isMapField(field core.OutputField) bool {
	desc := strings.ToLower(field.Description)
	if strings.Contains(desc, "map") || strings.Contains(desc, "dictionary") {
		return true
	}
	name := strings.ToLower(field.Name)
	if strings.Contains(name, "_scores") || strings.Contains(name, "_map") || strings.Contains(name, "_dict") {
		return true
	}
	return false
}

// defaultNewlineJoinedArrayField is always treated as repeated XML children coerced to one newline-joined string.
// Add more names via XMLConfig.ExtraArrayFieldsAsNewlineString / structured_output.Config.
const defaultNewlineJoinedArrayField = "usage_situations"

// newlineJoinedArrayFieldSet returns field names configured for array-in-XML, newline-joined Text output.
func newlineJoinedArrayFieldSet(config XMLConfig) map[string]struct{} {
	out := map[string]struct{}{
		defaultNewlineJoinedArrayField: {},
	}
	for _, n := range config.ExtraArrayFieldsAsNewlineString {
		if n != "" {
			out[n] = struct{}{}
		}
	}
	return out
}

// isArrayField determines if a field is expected to be a list/array (repeated child elements in XML).
// Used for fields like sub_questions with nested <question> or <item> elements.
func isArrayField(field core.OutputField, config XMLConfig) bool {
	if field.Type == core.FieldTypeText {
		if _, ok := newlineJoinedArrayFieldSet(config)[field.Name]; ok {
			return true
		}
	}
	desc := strings.ToLower(field.Description)
	if strings.Contains(desc, "list of") || strings.Contains(desc, "array") {
		return true
	}
	name := strings.ToLower(field.Name)
	// ExplanationGenerator: models emit <usage_situations><li>...</li></usage_situations>; plain string
	// fields ignore nested tags, so this must be collected as repeated children then joined to one string.
	if name == "usage_situations" {
		return true
	}
	if strings.HasSuffix(name, "_questions") || strings.HasSuffix(name, "_items") ||
		strings.HasSuffix(name, "_list") || strings.HasSuffix(name, "_spines") ||
		name == "sub_questions" {
		return true
	}
	return false
}

// findInstructionTargetField identifies the best input field for XML instructions.
func findInstructionTargetField(signature core.Signature) string {
	// Priority order for instruction injection.
	preferredFields := []string{"instruction", "prompt", "query", "question", "input"}

	// First, try preferred field names.
	for _, preferred := range preferredFields {
		for _, input := range signature.Inputs {
			if strings.EqualFold(input.Name, preferred) {
				return input.Name
			}
		}
	}

	// Fallback to first text field.
	for _, input := range signature.Inputs {
		if input.Type == core.FieldTypeText || input.Type == "" {
			return input.Name
		}
	}

	// Last resort: use first input field.
	if len(signature.Inputs) > 0 {
		return signature.Inputs[0].Name
	}

	return ""
}

// getTypeHint returns a type hint for XML instructions.
func getTypeHint(fieldType core.FieldType) string {
	switch fieldType {
	case core.FieldTypeText:
		return "[text content]"
	case core.FieldTypeImage:
		return "[image description or URL]"
	case core.FieldTypeAudio:
		return "[audio description or URL]"
	default:
		return "[content]"
	}
}
