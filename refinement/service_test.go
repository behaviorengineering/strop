package refinement

import (
	"testing"

	stroplog "github.com/behaviorengineering/strop/log"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type testLogger struct {
	entry *logrus.Entry
}

func (l *testLogger) WithField(key string, value interface{}) stroplog.Logger {
	return &testLogger{entry: l.entry.WithField(key, value)}
}

func (l *testLogger) WithFields(fields map[string]interface{}) stroplog.Logger {
	return &testLogger{entry: l.entry.WithFields(fields)}
}

func (l *testLogger) WithError(err error) stroplog.Logger {
	return &testLogger{entry: l.entry.WithError(err)}
}

func (l *testLogger) Debug(args ...interface{}) { l.entry.Debug(args...) }
func (l *testLogger) Info(args ...interface{})  { l.entry.Info(args...) }
func (l *testLogger) Warn(args ...interface{})  { l.entry.Warn(args...) }
func (l *testLogger) Error(args ...interface{}) { l.entry.Error(args...) }

func testRefinementService() ServiceInterface {
	return NewService(&testLogger{entry: logrus.NewEntry(logrus.New())}, "rejected", "pending", 1)
}

func TestCheckStoppingConditions_alignmentCapContinuesRefinement(t *testing.T) {
	s := testRefinementService()
	feedback := "[Content Strategist] READER QUALITY — required fix:\n[ ] tldr uses pun-domain contrast"
	stop, returnID := s.CheckStoppingConditions(4.0, 10.0, 28, uuid.New(), uuid.New(), feedback)
	if stop {
		t.Fatal("alignment quality cap should not stop refinement")
	}
	if returnID != uuid.Nil {
		t.Fatalf("expected nil return ID, got %v", returnID)
	}
}

func TestCheckStoppingConditions_scoreDecreaseStopsWithoutQualityCap(t *testing.T) {
	s := testRefinementService()
	selected := uuid.New()
	stop, returnID := s.CheckStoppingConditions(4.0, 10.0, 28, uuid.New(), selected, "[✓] All criteria met.")
	if !stop {
		t.Fatal("expected score decrease to stop refinement")
	}
	if returnID != selected {
		t.Fatalf("expected selected ID returned on decrease stop")
	}
}
