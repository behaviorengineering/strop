package jobskip

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memStore struct {
	byJob     map[string][]Record
	listErr   error
	unskipErr error
	unskipped []unskipCall
}

type unskipCall struct {
	rootID uuid.UUID
	job    string
}

func newMemStore() *memStore {
	return &memStore{byJob: map[string][]Record{}}
}

func (s *memStore) Skip(_ context.Context, rootID uuid.UUID, job string, reason *string) error {
	s.byJob[job] = append(s.byJob[job], Record{RootID: rootID, Job: job, Reason: reason, CreatedAt: time.Now()})
	return nil
}

func (s *memStore) Unskip(_ context.Context, rootID uuid.UUID, job string) error {
	if s.unskipErr != nil {
		return s.unskipErr
	}
	s.unskipped = append(s.unskipped, unskipCall{rootID: rootID, job: job})
	kept := s.byJob[job][:0]
	for _, rec := range s.byJob[job] {
		if rec.RootID != rootID {
			kept = append(kept, rec)
		}
	}
	s.byJob[job] = kept
	return nil
}

func (s *memStore) IsSkipped(_ context.Context, rootID uuid.UUID, job string) (bool, error) {
	for _, rec := range s.byJob[job] {
		if rec.RootID == rootID {
			return true, nil
		}
	}
	return false, nil
}

func (s *memStore) List(_ context.Context, job string) ([]Record, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := make([]Record, len(s.byJob[job]))
	copy(out, s.byJob[job])
	return out, nil
}

type staticLabeler struct {
	items []Item
	err   error
}

func (l staticLabeler) Labels(_ context.Context, records []Record) ([]Item, error) {
	if l.err != nil {
		return nil, l.err
	}
	if l.items != nil {
		return l.items, nil
	}
	items := make([]Item, 0, len(records))
	for _, rec := range records {
		items = append(items, Item{RootID: rec.RootID, Label: rec.RootID.String(), Reason: rec.Reason, CreatedAt: rec.CreatedAt})
	}
	return items, nil
}

type scriptedSelector struct {
	rootID  uuid.UUID
	proceed bool
	err     error
}

func (s scriptedSelector) Select(_ context.Context, _ []Item) (uuid.UUID, bool, error) {
	return s.rootID, s.proceed, s.err
}

func TestRestore_emptyList(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	result, err := Restore(context.Background(), store, "job_a", staticLabeler{}, scriptedSelector{proceed: true})
	require.NoError(t, err)
	assert.True(t, result.Empty)
	assert.Equal(t, uuid.Nil, result.RestoredID)
	assert.Empty(t, store.unskipped)
}

func TestRestore_selectAndUnskip(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	rootID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	require.NoError(t, store.Skip(context.Background(), rootID, "job_a", nil))

	result, err := Restore(context.Background(), store, "job_a", staticLabeler{}, scriptedSelector{rootID: rootID, proceed: true})
	require.NoError(t, err)
	assert.False(t, result.Empty)
	assert.Equal(t, rootID, result.RestoredID)
	require.Len(t, store.unskipped, 1)
	assert.Equal(t, rootID, store.unskipped[0].rootID)
	assert.Equal(t, "job_a", store.unskipped[0].job)

	skipped, err := store.IsSkipped(context.Background(), rootID, "job_a")
	require.NoError(t, err)
	assert.False(t, skipped)
}

func TestRestore_cancelDoesNotUnskip(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	rootID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	require.NoError(t, store.Skip(context.Background(), rootID, "job_a", nil))

	result, err := Restore(context.Background(), store, "job_a", staticLabeler{}, scriptedSelector{proceed: false})
	require.NoError(t, err)
	assert.False(t, result.Empty)
	assert.Equal(t, uuid.Nil, result.RestoredID)
	assert.Empty(t, store.unskipped)

	skipped, err := store.IsSkipped(context.Background(), rootID, "job_a")
	require.NoError(t, err)
	assert.True(t, skipped)
}

func TestRestore_listError(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	store.listErr = fmt.Errorf("db down")
	_, err := Restore(context.Background(), store, "job_a", staticLabeler{}, scriptedSelector{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list skipped roots")
	assert.ErrorContains(t, err, "db down")
}

func TestRestore_unskipError(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	rootID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	require.NoError(t, store.Skip(context.Background(), rootID, "job_a", nil))
	store.unskipErr = fmt.Errorf("delete failed")

	_, err := Restore(context.Background(), store, "job_a", staticLabeler{}, scriptedSelector{rootID: rootID, proceed: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unskip root")
	assert.ErrorContains(t, err, "delete failed")
}

func TestRestore_emptyJob(t *testing.T) {
	t.Parallel()
	_, err := Restore(context.Background(), newMemStore(), "", staticLabeler{}, scriptedSelector{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "job cannot be empty")
}

func TestRestore_nilStore(t *testing.T) {
	t.Parallel()
	_, err := Restore(context.Background(), nil, "job_a", staticLabeler{}, scriptedSelector{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store is required")
}

func TestRestore_labelerEmptyItems(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	rootID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	require.NoError(t, store.Skip(context.Background(), rootID, "job_a", nil))
	_, err := Restore(context.Background(), store, "job_a", staticLabeler{items: []Item{}}, scriptedSelector{rootID: rootID, proceed: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "labeler returned no items")
	assert.Empty(t, store.unskipped)
}
