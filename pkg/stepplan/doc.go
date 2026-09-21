// Package stepplan is the portable plan artifact and step-checkpoint contract.
//
// Hosts persist an ordered plan before execution, then checkpoint each completed
// step so resume skips finished work. This package does not run LLMs or own
// product vocabularies; orchestration.RunStepPlan (separate) drives execution.
//
// Filesystem layout (FileStore):
//
//	<root>/plans/<planID>/plan.json
//	<root>/plans/<planID>/steps/<stepID>.json
//
// The host supplies the root directory.
package stepplan
