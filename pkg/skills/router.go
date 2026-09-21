package skills

import (
	"slices"
	"strings"
)

// Request is the hard filter for one task.
// Allow empty selects nothing. SideEffects empty selects nothing.
// Cap zero uses DefaultSkillCap. Cap above MaxSkillCap is rejected.
type Request struct {
	Stage       string
	SideEffects []SideEffect
	Allow       []string
	Cap         int
}

// Selection is the subset bound for one task, in skill id order.
type Selection struct {
	Skills []Skill
}

// Select applies stage, side effect, and allow-list filters, then caps the result.
// Order is skill id order. That order is not a rank.
func Select(reg *Registry, req Request) (Selection, error) {
	if reg == nil {
		return Selection{}, invalid("nil registry")
	}
	stage := strings.TrimSpace(req.Stage)
	if stage == "" {
		return Selection{}, invalid("stage is required")
	}
	cap, err := normalizeCap(req.Cap, DefaultSkillCap, MaxSkillCap, "skill cap")
	if err != nil {
		return Selection{}, err
	}
	allowed := map[string]struct{}{}
	for _, id := range req.Allow {
		id = strings.TrimSpace(id)
		if id == "" {
			return Selection{}, invalid("allow list has an empty skill id")
		}
		allowed[id] = struct{}{}
	}
	effects := map[SideEffect]struct{}{}
	for _, effect := range req.SideEffects {
		if !knownSideEffect(effect) {
			return Selection{}, invalid("side effect is not recognized")
		}
		effects[effect] = struct{}{}
	}
	if len(allowed) == 0 || len(effects) == 0 {
		return Selection{}, nil
	}
	matched := make([]Skill, 0, len(allowed))
	for id := range allowed {
		skill, ok := reg.byID[id]
		if !ok {
			continue
		}
		if !stageAllows(skill, stage) {
			continue
		}
		if _, ok := effects[skill.SideEffect]; !ok {
			continue
		}
		skill.Stages = append([]string(nil), skill.Stages...)
		matched = append(matched, skill)
	}
	slices.SortFunc(matched, func(a, b Skill) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	if len(matched) > cap {
		matched = matched[:cap]
	}
	return Selection{Skills: matched}, nil
}

func stageAllows(skill Skill, stage string) bool {
	if len(skill.Stages) == 0 {
		return true
	}
	return slices.Contains(skill.Stages, stage)
}

func normalizeCap(cap, fallback, max int, label string) (int, error) {
	if cap == 0 {
		return fallback, nil
	}
	if cap < 0 || cap > max {
		return 0, invalid(label + " is out of range")
	}
	return cap, nil
}
