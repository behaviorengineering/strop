// Package trajectory provides durable, product-neutral pipeline trajectories
// over stepplan checkpoints and orchestration.RunStepPlan.
//
// Hosts supply a filesystem sidecar root, an Identity, and an ordered StepSpec
// list. Successful steps store opaque JSON output plus evaluation evidence.
// A later process resumes from the first incomplete or stale step without
// re-running scored work. Configuration paths (YAML, default log dirs) stay
// in the host; this package only receives an already-resolved Root.
package trajectory
