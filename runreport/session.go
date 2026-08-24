package runreport

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// StartSession begins a run report when enabled. Returns ctx with a collector and finish
// to call on exit (typically defer finish(err)).
func StartSession(ctx context.Context, cfg Config, meta Meta) (context.Context, func(error)) {
	cfg = cfg.Defaults()
	if !cfg.Enabled {
		return ctx, func(error) {}
	}
	if meta.RunID == "" {
		meta = NewMeta(meta.Pipeline, meta.Job, meta.EntityID, meta.Version)
	} else if meta.StartedAt.IsZero() {
		meta.StartedAt = time.Now().UTC()
	}
	collector := newCollector(meta)
	collector.Record(StepRunStart, "run started", map[string]interface{}{
		"pipeline":  meta.Pipeline,
		"job":       meta.Job,
		"entity_id": meta.EntityID,
		"version":   meta.Version,
		"run_id":    meta.RunID,
	})
	ctx = WithConfig(ctx, cfg)
	ctx = withCollector(ctx, collector)
	entityID := meta.EntityID
	finish := func(err error) {
		outcome := OutcomeSuccess
		errMsg := ""
		if err != nil {
			outcome = OutcomeFailed
			errMsg = err.Error()
		}
		report := collector.snapshot(outcome, errMsg)
		report.Steps = append(report.Steps, Step{
			At:      time.Now().UTC(),
			Kind:    StepRunFinish,
			Message: string(outcome),
			Error:   errMsg,
		})
		path, writeErr := WriteReport(cfg, report)
		if writeErr != nil {
			logError(writeErr, map[string]interface{}{
				"entity_id": entityID,
				"pipeline":  report.Meta.Pipeline,
				"job":       report.Meta.Job,
			}, "Failed to write run report")
			return
		}
		logInfo(map[string]interface{}{
			"entity_id": entityID,
			"path":      path,
			"pipeline":  report.Meta.Pipeline,
			"job":       report.Meta.Job,
			"outcome":   outcome,
		}, "Run report written")
		if pruneErr := Prune(cfg, entityID); pruneErr != nil {
			logError(pruneErr, map[string]interface{}{
				"entity_id": entityID,
				"path":      path,
			}, "Failed to prune old run reports")
		}
	}
	return ctx, finish
}

// WriteReport persists report as JSON and sets FilePath on the written document.
func WriteReport(cfg Config, report Report) (string, error) {
	cfg = cfg.Defaults()
	dir := reportDir(cfg, report.Meta)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create run report dir: %w", err)
	}
	filename := reportFilename(report.Meta)
	path := filepath.Join(dir, filename)
	report.FilePath = path
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal run report: %w", err)
	}
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return "", fmt.Errorf("write run report: %w", err)
	}
	return path, nil
}

func reportDir(cfg Config, meta Meta) string {
	pipeline := sanitizePathSegment(meta.Pipeline, "pipeline")
	job := sanitizePathSegment(meta.Job, "job")
	entity := sanitizePathSegment(meta.EntityID, "entity")
	return filepath.Join(cfg.Dir, pipeline, job, entity)
}

func reportFilename(meta Meta) string {
	ts := meta.StartedAt.UTC().Format("20060102T150405")
	runID := meta.RunID
	if len(runID) > 8 {
		runID = runID[:8]
	}
	runID = sanitizePathSegment(runID, "run")
	v := ""
	if meta.Version > 0 {
		v = fmt.Sprintf("-v%d", meta.Version)
	}
	return fmt.Sprintf("%s%s-%s.json", ts, v, runID)
}

func sanitizePathSegment(s, fallback string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback
	}
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, s)
	if s == "" {
		return fallback
	}
	return s
}

// ResolveMeta merges optional MetaProvider with entity ID and loop version.
func ResolveMeta(strategy interface{}, entityID string, version int) Meta {
	var m Meta
	if p, ok := strategy.(MetaProvider); ok {
		m = p.RunReportMeta()
	} else {
		m = Meta{Job: "refinement"}
	}
	if m.EntityID == "" {
		m.EntityID = entityID
	}
	if version > 0 {
		m.Version = version
	}
	if m.RunID == "" || m.StartedAt.IsZero() {
		base := NewMeta(m.Pipeline, m.Job, m.EntityID, m.Version)
		if m.RunID == "" {
			m.RunID = base.RunID
		}
		if m.StartedAt.IsZero() {
			m.StartedAt = base.StartedAt
		}
	}
	return m
}
