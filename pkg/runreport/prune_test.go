package runreport

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrune_TrimsOldReportsAndKeepsLatest(t *testing.T) {
	dir := t.TempDir()
	entityDir := filepath.Join(dir, "sayings", "post", "entity-x")
	if err := os.MkdirAll(entityDir, 0o750); err != nil {
		t.Fatal(err)
	}

	oldPath := filepath.Join(entityDir, "old.json")
	newPath := filepath.Join(entityDir, "new.json")
	if err := os.WriteFile(oldPath, []byte(`{}`), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte(`{}`), 0o640); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Enabled:       true,
		Dir:           dir,
		MaxAgeHours:   48,
		KeepPerEntity: 1,
	}
	if err := Prune(cfg, "entity-x"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("old report should be pruned by age")
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatal("new report should remain")
	}
}
