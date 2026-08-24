package runreport

import (
	kitlog "github.com/behaviorengineering/strop/log"
)

var defaultLogger kitlog.Logger

// SetDefaultLogger stores the app logger for run-report persistence diagnostics.
func SetDefaultLogger(logger kitlog.Logger) {
	defaultLogger = logger
}

func logInfo(fields map[string]interface{}, msg string) {
	if defaultLogger == nil {
		return
	}
	defaultLogger.WithFields(fields).Info(msg)
}

func logWarn(fields map[string]interface{}, msg string) {
	if defaultLogger == nil {
		return
	}
	defaultLogger.WithFields(fields).Warn(msg)
}

func logError(err error, fields map[string]interface{}, msg string) {
	if defaultLogger == nil {
		return
	}
	entry := defaultLogger.WithFields(fields)
	if err != nil {
		entry = entry.WithError(err)
	}
	entry.Error(msg)
}
