package reviewflow

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/behaviorengineering/strop/humanreview"
	"github.com/behaviorengineering/strop/regenerate"
)

// RegisterDefaultHandlers wires the live graph onto e using ports.
// Live states: Init, Generation, Alignment, Regeneration, FinalizeCriteria, Completion.
// Exit, Rejection, and Done are terminals (no handlers).
func RegisterDefaultHandlers(e *Engine, ports Ports, run *RunState) {
	if e == nil || run == nil {
		return
	}
	e.Register(StateInit, defaultHandleInit(ports, run))
	e.Register(StateGeneration, defaultHandleGeneration(ports, run))
	e.Register(StateAlignment, defaultHandleAlignment(ports, run))
	e.Register(StateRegeneration, defaultHandleRegeneration(ports, run))
	e.Register(StateFinalizeCriteria, defaultHandleFinalizeCriteria(ports, run))
	e.Register(StateCompletion, defaultHandleCompletion(ports, run))
}

func defaultHandleInit(ports Ports, run *RunState) Handler {
	return func(ctx context.Context) (State, error) {
		if run.EvaluationID == uuid.Nil {
			return StateGeneration, nil
		}
		if ports.Session == nil {
			return StateExit, fmt.Errorf("session port is required")
		}
		eval, err := ports.Session.Refresh(ctx, run.EvaluationID)
		if err != nil {
			return StateExit, err
		}
		if eval != nil && len(eval.PipelineHistory.History) > 0 {
			return StateAlignment, nil
		}
		return StateGeneration, nil
	}
}

func defaultHandleGeneration(ports Ports, run *RunState) Handler {
	return func(ctx context.Context) (State, error) {
		if ports.Generator == nil {
			return StateExit, fmt.Errorf("generator port is required")
		}
		res, err := ports.Generator.Generate(ctx, run.RootID, run.Job)
		if err != nil {
			return StateExit, err
		}
		if res == nil {
			return StateExit, fmt.Errorf("generation completed but no result returned")
		}
		run.ContentID = res.ContentID
		if len(run.CriterionIDs) == 0 {
			return StateDone, nil
		}
		if ports.Session == nil {
			return StateExit, fmt.Errorf("session port is required")
		}
		eval, err := ports.Session.StartEvaluation(ctx, run.RootID, run.Job, run.CriterionIDs)
		if err != nil {
			return StateExit, err
		}
		if eval == nil {
			return StateExit, fmt.Errorf("evaluation is nil after creation")
		}
		run.EvaluationID = eval.ID
		return StateAlignment, nil
	}
}

func defaultHandleAlignment(ports Ports, run *RunState) Handler {
	return func(ctx context.Context) (State, error) {
		if ports.Session == nil || ports.Prompter == nil {
			return StateExit, fmt.Errorf("session and prompter ports are required")
		}
		eval, err := ports.Session.Refresh(ctx, run.EvaluationID)
		if err != nil {
			return StateExit, err
		}
		agrees, comment, err := ports.Prompter.PromptAlignment(ctx, eval)
		if err != nil {
			return StateExit, err
		}
		if err := ports.Session.RecordAlignment(ctx, run.EvaluationID, agrees, comment); err != nil {
			return StateExit, err
		}
		if agrees {
			eval, err = ports.Session.Refresh(ctx, run.EvaluationID)
			if err != nil {
				return StateExit, err
			}
			if err := ports.Session.ProposeCriteria(ctx, eval, agrees, comment); err != nil {
				return StateExit, err
			}
			return StateFinalizeCriteria, nil
		}
		if comment != "" {
			useResearch, err := ports.Prompter.PromptRegenMode(ctx)
			if err != nil {
				return StateExit, err
			}
			run.UseResearch = useResearch
			n := ports.Normalizer
			if n == nil {
				n = humanreview.PassthroughNormalizer{}
			}
			msg, err := n.Normalize(regenerate.WithResearchMode(ctx, run.UseResearch), comment)
			if err != nil {
				return StateExit, err
			}
			run.LastStructuredFeedback = msg
		}
		if ports.Gate == nil {
			return StateExit, fmt.Errorf("approval gate is not available")
		}
		if err := ports.Gate.SetStatus(ctx, run.EvaluationID, humanreview.StatusRejected); err != nil {
			return StateExit, err
		}
		return StateRegeneration, nil
	}
}

func defaultHandleRegeneration(ports Ports, run *RunState) Handler {
	return func(ctx context.Context) (State, error) {
		if ports.Generator == nil || ports.Session == nil || ports.Gate == nil {
			return StateExit, fmt.Errorf("generator, session, and gate are required")
		}
		opts := regenerate.RegenerateOptions{
			Force:    true,
			Message:  run.LastStructuredFeedback,
			Research: run.UseResearch,
		}
		res, err := ports.Generator.Regenerate(ctx, run.RootID, run.Job, opts)
		if err != nil {
			if setErr := ports.Gate.SetStatus(ctx, run.EvaluationID, humanreview.StatusRejected); setErr != nil {
				return StateExit, fmt.Errorf("regeneration failed (%v) and persisting rejected status also failed: %w", err, setErr)
			}
			return StateRejection, nil
		}
		if res != nil {
			run.ContentID = res.ContentID
		}
		eval, err := ports.Session.Refresh(ctx, run.EvaluationID)
		if err != nil {
			return StateExit, err
		}
		if eval == nil {
			return StateExit, fmt.Errorf("evaluation is required after regeneration")
		}
		reopened, err := ports.Gate.Start(ctx, ports.PipelineType, eval.RootEntityID, eval.Job)
		if err != nil {
			return StateExit, err
		}
		if reopened == nil {
			return StateExit, fmt.Errorf("evaluation is required after reopening the approval gate")
		}
		run.EvaluationID = reopened.ID
		return StateAlignment, nil
	}
}

func defaultHandleFinalizeCriteria(ports Ports, run *RunState) Handler {
	return func(ctx context.Context) (State, error) {
		if ports.Session == nil {
			return StateExit, fmt.Errorf("session port is required")
		}
		if err := ports.Session.AutoAgreePending(ctx, run.EvaluationID); err != nil {
			return StateExit, err
		}
		return StateCompletion, nil
	}
}

func defaultHandleCompletion(ports Ports, run *RunState) Handler {
	return func(ctx context.Context) (State, error) {
		if ports.Session == nil {
			return StateExit, fmt.Errorf("session port is required")
		}
		eval, err := ports.Session.Refresh(ctx, run.EvaluationID)
		if err != nil {
			return StateExit, err
		}
		if err := ports.Session.ApprovePendingContent(ctx, eval); err != nil {
			return StateExit, err
		}
		if err := runLearnerAfterApproval(ctx, ports, run, eval); err != nil {
			return StateExit, err
		}
		return StateExit, nil
	}
}

func runLearnerAfterApproval(
	ctx context.Context,
	ports Ports,
	run *RunState,
	eval *humanreview.HumanEvaluation,
) error {
	if ports.Learner == nil {
		return nil
	}
	packs := ports.Packs
	if packs == nil {
		packs = humanreview.DefaultLearningPackRegistry()
	}
	pack, err := packs.Get(ports.PipelineType)
	if err != nil {
		return nil
	}
	if pack == nil || !pack.IsCompositionJob(run.Job) {
		return nil
	}
	if learnErr := ports.Learner.AfterApproval(ctx, eval); learnErr != nil {
		// Fail-open: approval already succeeded; log via engine is not available here.
		_ = learnErr
	}
	return nil
}
