// Package log defines the minimal logger interface used by strop packages.
// Apps adapt their concrete logger (e.g. observability.Logger) to this interface.
package log

// Logger is a structured logger used by kit packages (refinement, runreport diagnostics).
// WithField / WithFields / WithError must return a Logger so call chains stay on this interface.
type Logger interface {
	WithField(key string, value interface{}) Logger
	WithFields(fields map[string]interface{}) Logger
	WithError(err error) Logger
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
}
