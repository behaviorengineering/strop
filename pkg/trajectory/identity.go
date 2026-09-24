package trajectory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// DefinitionVersion bumps when the ordered step recipe for a job changes shape.
// Mismatched sidecars fail closed instead of silently resuming the wrong path.
const DefinitionVersion = 1

// Identity pins a trajectory to one entity/job run and its input fingerprint.
type Identity struct {
	Pipeline          string
	Job               string
	EntityID          string
	Version           int
	DefinitionVersion int
	SourceFingerprint string
	// Labels are optional product-neutral tags compared on reopen (e.g. arc id).
	Labels map[string]string
}

// Evidence is the opaque checkpoint payload for one successful step.
// Product-specific fields belong in Metadata, not first-class Evidence keys.
type Evidence struct {
	Score           float64            `json:"score,omitempty"`
	Feedback        string             `json:"feedback,omitempty"`
	Rationale       string             `json:"rationale,omitempty"`
	EvalRationale   string             `json:"eval_rationale,omitempty"`
	CriterionScores map[string]float64 `json:"criterion_scores,omitempty"`
	Metadata        map[string]string  `json:"metadata,omitempty"`
	Output          json.RawMessage    `json:"output"`
}

// PlanID returns a filesystem-safe plan id for the identity.
func PlanID(id Identity) string {
	pipeline := sanitizeID(id.Pipeline)
	job := sanitizeID(id.Job)
	entity := sanitizeID(id.EntityID)
	if pipeline == "" {
		pipeline = "pipeline"
	}
	if job == "" {
		job = "job"
	}
	if entity == "" {
		entity = "entity"
	}
	version := id.Version
	if version < 1 {
		version = 1
	}
	return fmt.Sprintf("%s_%s_%s_v%d", pipeline, job, entity, version)
}

// HashBytes returns a short hex digest for InputRef hashes.
func HashBytes(parts ...[]byte) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = h.Write(p)
		_, _ = h.Write([]byte{0})
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:16])
}

// HashStrings digests trimmed strings for source fingerprints.
func HashStrings(parts ...string) string {
	bufs := make([][]byte, 0, len(parts))
	for _, p := range parts {
		bufs = append(bufs, []byte(strings.TrimSpace(p)))
	}
	return HashBytes(bufs...)
}

func sanitizeID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	out = strings.Trim(out, "_")
	if out == "" {
		return "x"
	}
	return out
}

// MarshalEvidence encodes Evidence for a step checkpoint.
func MarshalEvidence(ev Evidence) (json.RawMessage, error) {
	if len(ev.Output) == 0 {
		return nil, fmt.Errorf("trajectory: evidence output is required")
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return nil, fmt.Errorf("trajectory: marshal evidence: %w", err)
	}
	return data, nil
}

// UnmarshalEvidence decodes a step checkpoint payload.
func UnmarshalEvidence(raw json.RawMessage) (Evidence, error) {
	var ev Evidence
	if len(raw) == 0 {
		return ev, fmt.Errorf("trajectory: empty evidence")
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		return ev, fmt.Errorf("trajectory: unmarshal evidence: %w", err)
	}
	if len(ev.Output) == 0 {
		return ev, fmt.Errorf("trajectory: evidence missing output")
	}
	return ev, nil
}
