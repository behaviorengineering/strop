package skills

import "strings"

// Trajectory records what one task was actually given.
// SkillIDs and BulletIDs are the injected set, not a guess about what the model used.
type Trajectory struct {
	TaskID    string
	Stage     string
	SkillIDs  []string
	BulletIDs []string
}

// Validate checks the record shape. An empty skill list is valid.
func (t Trajectory) Validate() error {
	if strings.TrimSpace(t.TaskID) == "" {
		return invalid("trajectory task id is required")
	}
	if strings.TrimSpace(t.Stage) == "" || strings.ContainsAny(t.Stage, " \t\n") {
		return invalid("trajectory stage must be a token")
	}
	if err := uniqueTokens(t.SkillIDs, "skill id"); err != nil {
		return err
	}
	if err := uniqueTokens(t.BulletIDs, "bullet id"); err != nil {
		return err
	}
	return nil
}

func uniqueTokens(ids []string, label string) error {
	seen := map[string]struct{}{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || strings.ContainsAny(id, " \t\n") {
			return invalid("trajectory " + label + " must be a token")
		}
		if _, ok := seen[id]; ok {
			return invalid("trajectory duplicate " + label + " " + id)
		}
		seen[id] = struct{}{}
	}
	return nil
}
