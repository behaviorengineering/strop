package jev

import (
	"fmt"

	"github.com/behaviorengineering/strop/pkg/evaluation/scoring"
)

// CloneBackend returns a new backend with the same configuration, or the original when not a JEV backend.
func CloneBackend(backend scoring.Backend) (scoring.Backend, error) {
	j, ok := backend.(*jevBackend)
	if !ok || j == nil {
		return backend, nil
	}
	cloned, err := NewBackend(j.client, j.criterionIDs, j.opts)
	if err != nil {
		return nil, fmt.Errorf("jev: clone backend: %w", err)
	}
	return cloned, nil
}
