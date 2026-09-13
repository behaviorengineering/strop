// Package ace wraps dspy-go Agentic Context Engineering as an ambient golang
// context.Context collaborator (same pattern as runreport and traces).
//
// Hosts open a session with NewManager (entity+job bound), attach it via
// WithManager, and Close when the session ends. Orchestration loops start and
// end trajectories from FromContext. JobRunner injects LearningsContext into
// generator inputs. A CatchInterceptor records module Process errors when a
// trajectory recorder is on ctx; it never ends the trajectory.
//
// ACE must not generalize across jobs or entities. Missing Manager means ACE is
// off (fail-open). RLM Complete is out of scope for this package; hosts may read
// FromContext at that edge later.
package ace
