package humanreview

import (
	"fmt"
	"strings"
)

// CandidateIdentityKey returns a stable dedup key for a learning candidate within one evaluation.
// Distinguishes per-section generator_example rows and per-principle content_rule rows
// that share the same job+step+type.
func CandidateIdentityKey(artifactType string, content map[string]interface{}) string {
	if content == nil {
		return artifactType
	}
	job, _ := content["job"].(string)
	step, _ := content["step"].(string)
	base := fmt.Sprintf("%s|%s|%s", artifactType, strings.TrimSpace(job), strings.TrimSpace(step))

	switch artifactType {
	case ArtifactTypeGeneratorExample:
		if section := sectionDiscriminator(content); section != "" {
			return base + "|section=" + section
		}
	case ArtifactTypeContentRule:
		if principle := principleDiscriminator(content); principle != "" {
			return base + "|principle=" + principle
		}
	case ArtifactTypeComponentAlignment, ArtifactTypeEvaluatorExample:
		if name, _ := content["criterion_name"].(string); strings.TrimSpace(name) != "" {
			return base + "|criterion=" + strings.TrimSpace(name)
		}
	}
	return base
}

func sectionDiscriminator(content map[string]interface{}) string {
	if ctx, ok := content["context"].(map[string]interface{}); ok {
		if section, _ := ctx["section_id"].(string); strings.TrimSpace(section) != "" {
			return strings.TrimSpace(section)
		}
	}
	if input, ok := content["input"].(map[string]interface{}); ok {
		if focus, _ := input["focus_section"].(string); strings.TrimSpace(focus) != "" {
			return strings.TrimSpace(focus)
		}
	}
	return ""
}

func principleDiscriminator(content map[string]interface{}) string {
	if principle, _ := content["principle"].(string); strings.TrimSpace(principle) != "" {
		return strings.TrimSpace(principle)
	}
	if rule, _ := content["rule"].(string); strings.TrimSpace(rule) != "" {
		return strings.TrimSpace(rule)
	}
	return ""
}

// HasCandidateIdentity reports whether any artifact matches the candidate identity key.
func HasCandidateIdentity(artifacts []*LearningArtifact, candidate LearningCandidate) bool {
	key := CandidateIdentityKey(candidate.Type, candidate.Content)
	for _, a := range artifacts {
		if a == nil {
			continue
		}
		if CandidateIdentityKey(a.ArtifactType, a.ArtifactContent) == key {
			return true
		}
	}
	return false
}
