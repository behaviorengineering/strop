package humanreview

import "context"

// ScoreProposal is a criterion score suggestion from an injectable proposer.
type ScoreProposal struct {
	ProposedScore float64
	Rationale     string
	Analysis      string
	Evidence      []string
}

// PreviousScoreProposal is the last AI proposal, used when refining after disagreement.
type PreviousScoreProposal struct {
	Score     float64
	Rationale string
}

// HumanScoreFeedback is the reviewer's score and comment for a criterion.
type HumanScoreFeedback struct {
	Score   *float64
	Comment string
}

// AlignmentFeedback is the reviewer's overall alignment with the artifact.
type AlignmentFeedback struct {
	Agrees  bool
	Comment string
}

// ScoreProposalInputs is the portable request for Propose.
type ScoreProposalInputs struct {
	CriterionName        string
	CriterionDescription string
	History              PipelineHistory
	Previous             *PreviousScoreProposal
	HumanFeedback        *HumanScoreFeedback
	Alignment            *AlignmentFeedback
}

// ScoreProposer proposes a criterion score. Apps inject a DSPy adapter; the kit does not import DSPy.
type ScoreProposer interface {
	Propose(ctx context.Context, in ScoreProposalInputs) (*ScoreProposal, error)
}
