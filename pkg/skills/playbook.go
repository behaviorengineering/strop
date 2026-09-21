package skills

import (
	"slices"
	"strings"
)

// Polarity labels playbook advice. It is not a skill.
type Polarity string

const (
	PolarityHelpful Polarity = "helpful"
	PolarityHarmful Polarity = "harmful"
)

// Bullet is one piece of advice the host may inject into a task.
type Bullet struct {
	ID       string
	Scope    string
	Text     string
	Polarity Polarity
}

// BulletRequest caps one scope's advice.
// Caps of zero use DefaultBulletCap. A cap above MaxBulletCap is rejected.
type BulletRequest struct {
	Scope      string
	HelpfulCap int
	HarmfulCap int
}

// SelectBullets keeps bullets in the requested scope, split by polarity, in id order.
func SelectBullets(bullets []Bullet, req BulletRequest) ([]Bullet, error) {
	const op = "skills.SelectBullets"
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		return nil, invalid(op, "playbook scope is required")
	}
	helpfulCap, err := normalizeCap(op, req.HelpfulCap, DefaultBulletCap, MaxBulletCap, "helpful cap")
	if err != nil {
		return nil, err
	}
	harmfulCap, err := normalizeCap(op, req.HarmfulCap, DefaultBulletCap, MaxBulletCap, "harmful cap")
	if err != nil {
		return nil, err
	}
	var helpful, harmful []Bullet
	for _, bullet := range bullets {
		cleaned, err := normalizeBullet(op, bullet)
		if err != nil {
			return nil, err
		}
		if cleaned.Scope != scope {
			continue
		}
		switch cleaned.Polarity {
		case PolarityHelpful:
			helpful = append(helpful, cleaned)
		case PolarityHarmful:
			harmful = append(harmful, cleaned)
		}
	}
	sortBullets(helpful)
	sortBullets(harmful)
	if len(helpful) > helpfulCap {
		helpful = helpful[:helpfulCap]
	}
	if len(harmful) > harmfulCap {
		harmful = harmful[:harmfulCap]
	}
	out := make([]Bullet, 0, len(helpful)+len(harmful))
	out = append(out, helpful...)
	out = append(out, harmful...)
	return out, nil
}

func normalizeBullet(op string, b Bullet) (Bullet, error) {
	b.ID = strings.TrimSpace(b.ID)
	if b.ID == "" || len(b.ID) > maxIDLen || strings.ContainsAny(b.ID, " \t\n") {
		return Bullet{}, invalid(op, "bullet id must be a token")
	}
	b.Scope = strings.TrimSpace(b.Scope)
	if b.Scope == "" || strings.ContainsAny(b.Scope, " \t\n") {
		return Bullet{}, invalid(op, "bullet scope must be a token")
	}
	b.Text = strings.TrimSpace(b.Text)
	if b.Text == "" || len(b.Text) > maxBulletText {
		return Bullet{}, invalid(op, "bullet text must be a short line")
	}
	switch b.Polarity {
	case PolarityHelpful, PolarityHarmful:
	default:
		return Bullet{}, invalid(op, "bullet polarity is not recognized")
	}
	return b, nil
}

func sortBullets(bullets []Bullet) {
	slices.SortFunc(bullets, func(a, b Bullet) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
}
