// Package voice is the portable reader-prose voice judge for strop consumers.
//
// The package scores cadence and caller-supplied phrase banks. It does not name
// a product, a persona, or a site section. Attach the evaluator on polish jobs
// that modulate voice after a neutral composition. Prefer a cheap model via roleProviders.
package voice

import (
	"github.com/behaviorengineering/strop/pkg/evaluation"
	"github.com/behaviorengineering/strop/pkg/evaluation/criteria"
)

// EvaluatorKey is the parallel evaluation role for voice fidelity.
const EvaluatorKey evaluation.EvaluatorKey = "voice_fidelity"

// DefaultMaxStaccatoRun is the longest run of similar short sentences that still passes.
// A longer run fails. Zero on a Profile means this default.
const DefaultMaxStaccatoRun = 2

// Profile is a caller-supplied voice contract. Phrase banks stay with the caller.
type Profile struct {
	ID             string
	Label          string
	BannedPatterns []string
	RequiredVerbs  []string
	MaxStaccatoRun int
	ToneNotes      string
	// Example is optional sample prose the generator may match for cadence.
	Example string
}

// Violation is one deterministic miss.
type Violation struct {
	Kind    string
	Line    int
	Excerpt string
	Detail  string
}

// AuditResult is the heuristic verdict. Pass is true when Violations is empty.
type AuditResult struct {
	Pass                   bool
	Violations             []Violation
	LongestStaccatoRun     int
	ParagraphsMissingVerbs int
}

// CriterionIDs returns the criterion set owned by the voice role.
func CriterionIDs() []criteria.CriterionID {
	return []criteria.CriterionID{criteria.CriterionIDVoiceFidelity}
}

func (p Profile) maxStaccatoRun() int {
	if p.MaxStaccatoRun <= 0 {
		return DefaultMaxStaccatoRun
	}
	return p.MaxStaccatoRun
}
