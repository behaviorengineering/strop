// Package skills is a portable skill registry and hard-filter router.
// A skill is a named capability the host may invoke. A playbook bullet is advice, not a skill.
// The router only subsets a registry. It does not create skills and it does not rank them.
package skills

import (
	"regexp"
	"strings"
)

const (
	// MaxSkillCap is the most skills one selection may return.
	MaxSkillCap = 5
	// DefaultSkillCap is used when Request.Cap is zero.
	DefaultSkillCap = MaxSkillCap
	// MaxBulletCap is the most bullets of one polarity one slice may return.
	MaxBulletCap = 5
	// DefaultBulletCap is used when a polarity cap is zero.
	DefaultBulletCap = MaxBulletCap
	maxDescription   = 200
	maxVersionLen    = 32
	maxBulletText    = 280
	maxIDLen         = 64
)

// SideEffect is the closed set of effects a skill may have.
// Hosts map their own classes onto these three values.
type SideEffect string

const (
	SideEffectNone  SideEffect = "none"
	SideEffectRead  SideEffect = "read"
	SideEffectWrite SideEffect = "write"
)

var skillIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)+$`)

// Skill is one registered capability.
// Stages empty means the skill may be offered at every stage.
type Skill struct {
	ID          string
	Version     string
	Description string
	SideEffect  SideEffect
	Stages      []string
}

// Registry holds skills by id. It does not persist them.
type Registry struct {
	byID map[string]Skill
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byID: map[string]Skill{}}
}

// Register copies s into the registry. A second register of the same id fails.
func (r *Registry) Register(s Skill) error {
	const op = "skills.Registry.Register"
	if r == nil {
		return invalid(op, "nil registry")
	}
	cleaned, err := normalizeSkill(op, s)
	if err != nil {
		return err
	}
	if _, exists := r.byID[cleaned.ID]; exists {
		return invalidField(op, "duplicate skill id", "id", cleaned.ID)
	}
	r.byID[cleaned.ID] = cleaned
	return nil
}

// Get returns a copy of the skill.
func (r *Registry) Get(id string) (Skill, bool) {
	if r == nil {
		return Skill{}, false
	}
	s, ok := r.byID[strings.TrimSpace(id)]
	if !ok {
		return Skill{}, false
	}
	s.Stages = append([]string(nil), s.Stages...)
	return s, true
}

// Len reports how many skills are registered.
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.byID)
}

func normalizeSkill(op string, s Skill) (Skill, error) {
	s.ID = strings.TrimSpace(s.ID)
	if !skillIDPattern.MatchString(s.ID) || len(s.ID) > maxIDLen {
		return Skill{}, invalid(op, "skill id must be a dotted token")
	}
	s.Version = strings.TrimSpace(s.Version)
	if s.Version == "" || len(s.Version) > maxVersionLen || strings.ContainsAny(s.Version, " \t\n") {
		return Skill{}, invalid(op, "skill version is required")
	}
	s.Description = strings.TrimSpace(s.Description)
	if s.Description == "" || len(s.Description) > maxDescription {
		return Skill{}, invalid(op, "skill description must be a short sentence")
	}
	switch s.SideEffect {
	case SideEffectNone, SideEffectRead, SideEffectWrite:
	default:
		return Skill{}, invalid(op, "skill side effect is not recognized")
	}
	stages := make([]string, 0, len(s.Stages))
	seen := map[string]struct{}{}
	for _, stage := range s.Stages {
		stage = strings.TrimSpace(stage)
		if stage == "" || strings.ContainsAny(stage, " \t\n") {
			return Skill{}, invalid(op, "skill stage must be a token")
		}
		if _, ok := seen[stage]; ok {
			continue
		}
		seen[stage] = struct{}{}
		stages = append(stages, stage)
	}
	s.Stages = stages
	return s, nil
}

func knownSideEffect(effect SideEffect) bool {
	switch effect {
	case SideEffectNone, SideEffectRead, SideEffectWrite:
		return true
	default:
		return false
	}
}
