package factory

import (
	"context"
	"errors"
	"fmt"

	kitdspy "github.com/behaviorengineering/strop/dspy"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/XiaoConstantine/dspy-go/pkg/modules"
)

// FeedbackFactory creates feedback analysis modules with any role type using Go generics.
// This allows the same factory to work with sayings, videos, or any future pipeline's feedback roles.
type FeedbackFactory struct {
	configurator *ModuleConfigurator
}

// NewFeedbackFactory creates a new feedback factory.
func NewFeedbackFactory(configurator *ModuleConfigurator) *FeedbackFactory {
	if configurator == nil {
		panic("configurator cannot be nil")
	}
	return &FeedbackFactory{
		configurator: configurator,
	}
}

// CreateFeedbackAnalysisModule creates a feedback analysis module for a specific role.
//
// R is a generic role type that must be comparable.
// The pipeline provides:
//   - role: The role to create a module for
//   - getRoleName: Function to get display name for a role
//   - rolePrompts: Map of role to system prompt
//   - createModule: Function to create a module for a role name and prompt
func CreateFeedbackAnalysisModule[R comparable](
	f *FeedbackFactory,
	ctx context.Context,
	provider kitdspy.ProviderConfig,
	role R,
	getRoleName func(R) string,
	rolePrompts map[R]string,
	createModule func(roleName string, systemPrompt string) (core.Module, error),
	errorPrefix string,
) (core.Module, error) {
	roleName := getRoleName(role)

	// Get prompt from pipeline-provided map.
	prompt, ok := rolePrompts[role]
	if !ok {
		return nil, fmt.Errorf("no prompt for feedback analysis role: %v", role)
	}

	// Create module using pipeline-provided function.
	module, err := createModule(roleName, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to create feedback analysis module for role %v: %w", role, err)
	}

	// Setup module (LLM, XML output, interceptors).
	if err := f.configurator.SetupModule(ctx, provider, module, errorPrefix); err != nil {
		return nil, err
	}

	return module, nil
}

// CreateFeedbackFormatterModule creates a feedback formatter module.
//
// R is a generic role type that must be comparable.
// The pipeline provides:
//   - formatterRole: The role to use for the formatter
//   - getRoleName: Function to get display name for a role
//   - formatterPrompt: System prompt for the formatter
//   - createModule: Function to create a Predict module for a role name and prompt
func CreateFeedbackFormatterModule[R comparable](
	f *FeedbackFactory,
	ctx context.Context,
	provider kitdspy.ProviderConfig,
	formatterRole R,
	getRoleName func(R) string,
	formatterPrompt string,
	createModule func(roleName string, systemPrompt string) (*modules.Predict, error),
	errorPrefix string,
) (*modules.Predict, error) {
	if formatterPrompt == "" {
		return nil, errors.New("feedback formatter prompt is required")
	}

	roleName := getRoleName(formatterRole)

	// Create Predict module using pipeline-provided function.
	predict, err := createModule(roleName, formatterPrompt)
	if err != nil {
		return nil, fmt.Errorf(ErrModuleCreationFailed, errorPrefix, err)
	}

	if err := f.configurator.SetupModule(ctx, provider, predict, errorPrefix); err != nil {
		return nil, err
	}

	return predict, nil
}
