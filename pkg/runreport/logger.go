package runreport

import (
	"sync"

	stroplog "github.com/behaviorengineering/strop/pkg/log"
)

var (
	loggerMu      sync.RWMutex
	defaultLogger stroplog.Logger
)

// SetDefaultLogger stores the process-wide logger used when a run report has no
// caller-supplied logger. Hosts should set it once at startup. Concurrent calls
// are safe. Tests that change it should restore the previous logger.
func SetDefaultLogger(logger stroplog.Logger) {
	loggerMu.Lock()
	defaultLogger = logger
	loggerMu.Unlock()
}

func currentLogger() stroplog.Logger {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	return defaultLogger
}

func logInfo(fields map[string]interface{}, msg string) {
	logger := currentLogger()
	if logger == nil {
		return
	}
	logger.WithFields(fields).Info(msg)
}

func logError(err error, fields map[string]interface{}, msg string) {
	logger := currentLogger()
	if logger == nil {
		return
	}
	entry := logger.WithFields(fields)
	if err != nil {
		entry = entry.WithError(err)
	}
	entry.Error(msg)
}
