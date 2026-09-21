package ace

import "context"

type managerKey struct{}
type recorderKey struct{}

// WithManager attaches a session Manager to ctx. Nil Manager is a no-op.
func WithManager(ctx context.Context, m *Manager) context.Context {
	if m == nil {
		return ctx
	}
	return context.WithValue(ctx, managerKey{}, m)
}

// FromContext returns the session Manager, or nil when ACE is off.
func FromContext(ctx context.Context) *Manager {
	if ctx == nil {
		return nil
	}
	m, ok := ctx.Value(managerKey{}).(*Manager)
	if !ok {
		return nil
	}
	return m
}

// WithRecorder attaches the active trajectory recorder for catch interceptors.
func WithRecorder(ctx context.Context, r *TrajectoryRecorder) context.Context {
	if r == nil {
		return ctx
	}
	return context.WithValue(ctx, recorderKey{}, r)
}

// RecorderFromContext returns the active trajectory recorder, or nil.
func RecorderFromContext(ctx context.Context) *TrajectoryRecorder {
	if ctx == nil {
		return nil
	}
	r, ok := ctx.Value(recorderKey{}).(*TrajectoryRecorder)
	if !ok {
		return nil
	}
	return r
}
