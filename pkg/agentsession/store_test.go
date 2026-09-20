package agentsession

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateLoadAppendSaveClosePrune(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	meta, err := store.Create(ctx, "prune_investigate", map[string]any{
		"branch": "feat/gone",
	})
	if err != nil {
		t.Fatal(err)
	}
	if meta.ID == "" || meta.Status != StatusOpen {
		t.Fatalf("meta: %+v", meta)
	}
	dir, err := store.Dir(meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("session must be a directory: %v", err)
	}

	loaded, err := store.Load(ctx, meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Extra["branch"] != "feat/gone" {
		t.Fatalf("extra: %+v", loaded.Extra)
	}

	if err := store.AppendTurn(ctx, meta.ID, Turn{Role: "user", Content: "investigate"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTurn(ctx, meta.ID, Turn{Role: "assistant", Content: "drop", Refs: map[string]any{"card": true}}); err != nil {
		t.Fatal(err)
	}
	turns, err := store.ReadTurns(ctx, meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 {
		t.Fatalf("turns=%d", len(turns))
	}

	card := map[string]any{"verdict": "drop"}
	if err := store.SaveJSON(ctx, meta.ID, FileCard, card); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := store.LoadJSON(ctx, meta.ID, FileCard, &got); err != nil {
		t.Fatal(err)
	}
	if got["verdict"] != "drop" {
		t.Fatalf("card: %+v", got)
	}

	if err := store.Close(ctx, meta.ID); err != nil {
		t.Fatal(err)
	}
	closed, err := store.Load(ctx, meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != StatusClosed {
		t.Fatalf("status=%q", closed.Status)
	}
	if err := store.AppendTurn(ctx, meta.ID, Turn{Role: "user", Content: "nope"}); err == nil {
		t.Fatal("expected append on closed session to fail")
	}

	if err := store.Prune(ctx, PruneOpts{ClosedOnly: true, MaxAge: time.Nanosecond}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected pruned dir gone, err=%v", err)
	}
}

func TestListFiltersAndKeepPerKind(t *testing.T) {
	t.Parallel()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	a, err := store.Create(ctx, "prune_investigate", nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	b, err := store.Create(ctx, "prune_investigate", nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	c, err := store.Create(ctx, "ci_triage", nil)
	if err != nil {
		t.Fatal(err)
	}

	listed, err := store.List(ctx, ListOpts{Kind: "prune_investigate"})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed=%d", len(listed))
	}
	if listed[0].ID != b.ID {
		t.Fatalf("want newest first, got %s want %s", listed[0].ID, b.ID)
	}

	if err := store.Prune(ctx, PruneOpts{KeepPerKind: 1}); err != nil {
		t.Fatal(err)
	}
	left, err := store.List(ctx, ListOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 2 {
		t.Fatalf("after keep=1 expect 2 (one prune + one ci), got %d", len(left))
	}
	ids := map[string]bool{}
	for _, m := range left {
		ids[m.ID] = true
	}
	if !ids[b.ID] || !ids[c.ID] || ids[a.ID] {
		t.Fatalf("keep newest per kind: %+v", ids)
	}
}

func TestRejectPathTraversal(t *testing.T) {
	t.Parallel()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := store.Load(ctx, "../etc"); err == nil {
		t.Fatal("expected invalid id")
	}
	meta, err := store.Create(ctx, "k", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveJSON(ctx, meta.ID, "../x.json", map[string]any{}); err == nil {
		t.Fatal("expected invalid blob name")
	}
	if err := store.SaveJSON(ctx, meta.ID, FileMeta, map[string]any{}); err == nil {
		t.Fatal("expected reserved meta name")
	}
}

func TestNewRequiresRoot(t *testing.T) {
	t.Parallel()
	if _, err := New(""); err == nil {
		t.Fatal("expected error")
	}
}

func TestSessionDirLayout(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := store.Create(context.Background(), "k", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "sessions", meta.ID)
	got, err := store.Dir(meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("dir=%s want %s", got, want)
	}
	entries, err := os.ReadDir(want)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != FileMeta {
		t.Fatalf("entries=%v", entries)
	}
}
