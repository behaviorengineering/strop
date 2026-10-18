package trajectory

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/behaviorengineering/strop/pkg/stepplan"
)

var planVersionSuffix = regexp.MustCompile(`_v(\d+)$`)

// Inspect loads an existing plan and step checkpoints without creating or mutating files.
// Missing plan returns nil, nil. Corrupt plan returns an error.
func Inspect(ctx context.Context, root, planID string) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root = strings.TrimSpace(root)
	planID = strings.TrimSpace(planID)
	if root == "" || planID == "" {
		return nil, nil
	}
	store, err := stepplan.NewFileStore(root)
	if err != nil {
		return nil, fmt.Errorf("trajectory: inspect file store: %w", err)
	}
	plan, loadErr := store.LoadPlan(ctx, planID)
	if loadErr != nil {
		if isNotFound(loadErr) {
			return nil, nil
		}
		return nil, fmt.Errorf("trajectory: inspect load plan: %w", loadErr)
	}
	planDir, err := store.PlanDir(planID)
	if err != nil {
		return nil, fmt.Errorf("trajectory: inspect plan dir: %w", err)
	}
	id := identityFromPlanExtra(plan.Extra)
	return &Session{
		Store:   store,
		Plan:    plan,
		Root:    root,
		PlanDir: planDir,
		ID:      id,
		Resumed: true,
	}, nil
}

// ListPlanIDs returns plan directory names under root/plans that start with prefix.
func ListPlanIDs(ctx context.Context, root, prefix string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root = strings.TrimSpace(root)
	prefix = strings.TrimSpace(prefix)
	if root == "" {
		return nil, nil
	}
	store, err := stepplan.NewFileStore(root)
	if err != nil {
		return nil, fmt.Errorf("trajectory: list file store: %w", err)
	}
	entries, err := os.ReadDir(store.PlansDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("trajectory: list plans dir: %w", err)
	}
	out := make([]string, 0, len(entries))
	for _, ent := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !ent.IsDir() {
			continue
		}
		name := ent.Name()
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// HighestVersionPlanID picks the plan id with the greatest _vN suffix among prefix matches.
// Returns empty string when none exist.
func HighestVersionPlanID(ctx context.Context, root, prefix string) (string, error) {
	ids, err := ListPlanIDs(ctx, root, prefix)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", nil
	}
	best := ids[0]
	bestV := planIDVersion(best)
	for _, id := range ids[1:] {
		v := planIDVersion(id)
		if v > bestV {
			best = id
			bestV = v
		}
	}
	return best, nil
}

func planIDVersion(planID string) int {
	m := planVersionSuffix.FindStringSubmatch(planID)
	if len(m) < 2 {
		return 0
	}
	var v int
	for _, ch := range m[1] {
		if ch < '0' || ch > '9' {
			return 0
		}
		v = v*10 + int(ch-'0')
	}
	return v
}

func identityFromPlanExtra(extra map[string]any) Identity {
	if extra == nil {
		return Identity{}
	}
	ver, _ := intField(extra, extraVersionKey)
	defVer, _ := intField(extra, extraDefinitionVersionKey)
	id := Identity{
		Pipeline:          stringField(extra, extraPipelineKey),
		Job:               stringField(extra, extraJobKey),
		EntityID:          stringField(extra, extraEntityKey),
		SourceFingerprint: stringField(extra, extraSourceFingerprintKey),
		Version:           ver,
		DefinitionVersion: defVer,
	}
	labels := make(map[string]string)
	for k, v := range extra {
		if !strings.HasPrefix(k, extraLabelPrefix) {
			continue
		}
		labels[strings.TrimPrefix(k, extraLabelPrefix)] = strings.TrimSpace(fmt.Sprint(v))
	}
	if len(labels) > 0 {
		id.Labels = labels
	}
	return id
}
