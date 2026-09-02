// Package concurrency provides adaptive in-flight limiting for parallel LLM or
// I/O work pools. Use RunPool when many independent units can run concurrently
// but backend capacity is unknown; prefer orchestration per-item refinement
// loops when each unit needs sequential generate-evaluate feedback.
package concurrency
