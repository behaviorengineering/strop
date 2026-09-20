package runreport

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartSession_WritesReportWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Enabled:       true,
		Dir:           dir,
		MaxAgeHours:   48,
		KeepPerEntity: 5,
	}
	meta := NewMeta("sayings", "post", "entity-1", 2)

	ctx, finish := StartSession(context.Background(), cfg, meta)
	CollectorFromContext(ctx).Record(StepWarning, "parse retry", nil)
	finish(nil)

	matches, err := filepath.Glob(filepath.Join(dir, "**", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		// Walk manually — glob may not recurse on all platforms.
		var found bool
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && filepath.Ext(path) == ".json" {
				found = true
			}
			return nil
		})
		if !found {
			t.Fatal("expected a JSON run report file")
		}
	}
}

func TestStartSession_NoOpWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{Enabled: false, Dir: dir}
	meta := NewMeta("sayings", "post", "entity-1", 1)

	ctx, finish := StartSession(context.Background(), cfg, meta)
	if CollectorFromContext(ctx) != nil {
		t.Fatal("disabled session should not attach collector")
	}
	finish(nil)

	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) > 0 {
		t.Fatal("disabled session should not write files")
	}
}

func TestResolveMeta_WithVersion(t *testing.T) {
	type stub struct{}
	meta := ResolveMeta(stub{}, "entity-1", 5)
	if meta.Version != 5 {
		t.Fatalf("version = %d, want 5", meta.Version)
	}
	if meta.EntityID != "entity-1" {
		t.Fatalf("entity_id = %q", meta.EntityID)
	}
}

func TestWriteReport_SetsFilePath(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{Dir: dir}
	meta := NewMeta("sayings", "post", "entity-1", 2)
	report := Report{Meta: meta, Outcome: OutcomeSuccess, Steps: []Step{}}
	path, err := WriteReport(cfg, report)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"file_path"`) {
		t.Fatal("expected file_path in written JSON")
	}
}
