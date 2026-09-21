package humanreview

import (
	"fmt"
	"strings"
)

// StringField returns a trimmed string field.
// A missing or nil field is empty with a nil error.
// A present value that is not a string is a validation error.
func StringField(content map[string]interface{}, key string) (string, error) {
	if content == nil {
		return "", nil
	}
	raw, exists := content[key]
	if !exists || raw == nil {
		return "", nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("humanreview: content field %q must be a string", key)
	}
	return strings.TrimSpace(s), nil
}

// ObjectField returns a nested object field.
// A missing or nil field is nil with a nil error.
// A present value that is not an object is a validation error.
func ObjectField(content map[string]interface{}, key string) (map[string]interface{}, error) {
	if content == nil {
		return nil, nil
	}
	raw, exists := content[key]
	if !exists || raw == nil {
		return nil, nil
	}
	obj, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("humanreview: content field %q must be an object", key)
	}
	return obj, nil
}

// CandidateIdentityKey returns a stable dedup key for a learning candidate within one evaluation.
// Distinguishes per-section generator_example rows and per-principle content_rule rows
// that share the same job, step, and type.
func CandidateIdentityKey(artifactType string, content map[string]interface{}) (string, error) {
	if content == nil {
		return artifactType, nil
	}
	job, err := StringField(content, "job")
	if err != nil {
		return "", err
	}
	step, err := StringField(content, "step")
	if err != nil {
		return "", err
	}
	base := fmt.Sprintf("%s|%s|%s", artifactType, job, step)

	switch artifactType {
	case ArtifactTypeGeneratorExample:
		section, err := sectionDiscriminator(content)
		if err != nil {
			return "", err
		}
		if section != "" {
			return base + "|section=" + section, nil
		}
	case ArtifactTypeContentRule:
		principle, err := principleDiscriminator(content)
		if err != nil {
			return "", err
		}
		if principle != "" {
			return base + "|principle=" + principle, nil
		}
	case ArtifactTypeComponentAlignment, ArtifactTypeEvaluatorExample:
		name, err := StringField(content, "criterion_name")
		if err != nil {
			return "", err
		}
		if name != "" {
			return base + "|criterion=" + name, nil
		}
	}
	return base, nil
}

func sectionDiscriminator(content map[string]interface{}) (string, error) {
	ctx, err := ObjectField(content, "context")
	if err != nil {
		return "", err
	}
	if section, err := StringField(ctx, "section_id"); err != nil || section != "" {
		return section, err
	}
	input, err := ObjectField(content, "input")
	if err != nil {
		return "", err
	}
	return StringField(input, "focus_section")
}

func principleDiscriminator(content map[string]interface{}) (string, error) {
	principle, err := StringField(content, "principle")
	if err != nil || principle != "" {
		return principle, err
	}
	return StringField(content, "rule")
}

// HasCandidateIdentity reports whether any artifact matches the candidate identity key.
// Malformed content does not match.
func HasCandidateIdentity(artifacts []*LearningArtifact, candidate LearningCandidate) bool {
	key, err := CandidateIdentityKey(candidate.Type, candidate.Content)
	if err != nil {
		return false
	}
	for _, a := range artifacts {
		if a == nil {
			continue
		}
		other, err := CandidateIdentityKey(a.ArtifactType, a.ArtifactContent)
		if err != nil {
			continue
		}
		if other == key {
			return true
		}
	}
	return false
}
