package runreport

import (
	"sync"
	"time"
)

// Collector accumulates steps for one run. Safe for concurrent use.
type Collector struct {
	mu    sync.Mutex
	meta  Meta
	steps []Step
}

func newCollector(meta Meta) *Collector {
	return &Collector{meta: meta, steps: make([]Step, 0, 32)}
}

func (c *Collector) Meta() Meta {
	if c == nil {
		return Meta{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.meta
}

func (c *Collector) append(step Step) {
	if c == nil {
		return
	}
	if step.At.IsZero() {
		step.At = time.Now().UTC()
	}
	c.mu.Lock()
	c.steps = append(c.steps, step)
	c.mu.Unlock()
}

func (c *Collector) snapshot(outcome Outcome, errMsg string) Report {
	if c == nil {
		return Report{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	steps := make([]Step, len(c.steps))
	copy(steps, c.steps)
	return Report{
		Meta:    c.meta,
		Outcome: outcome,
		Error:   errMsg,
		Steps:   steps,
	}
}

func (c *Collector) Record(kind StepKind, message string, details map[string]interface{}) {
	c.append(Step{Kind: kind, Message: message, Details: details})
}

func (c *Collector) RecordPhase(phase string, attempt int, passed bool, score float64, message string) {
	ok := passed
	c.append(Step{
		Kind:    StepCompositionPhase,
		Phase:   phase,
		Attempt: attempt,
		OK:      &ok,
		Score:   &score,
		Message: message,
	})
}

func (c *Collector) RecordRefinement(version int, score float64, message string) {
	c.append(Step{
		Kind:    StepRefinementIteration,
		Attempt: version,
		Score:   &score,
		Message: message,
	})
}

func (c *Collector) RecordPerItemRefinement(itemIndex, round int, score float64, message string) {
	c.append(Step{
		Kind:    StepRefinementIteration,
		Attempt: round,
		Score:   &score,
		Message: message,
		Details: map[string]interface{}{
			"item_index":  itemIndex,
			"item_number": itemIndex + 1,
		},
	})
}

func (c *Collector) RecordModule(module string, ok bool, err error, duration time.Duration) {
	b := ok
	step := Step{
		Kind:       StepModuleCall,
		Module:     module,
		OK:         &b,
		DurationMS: duration.Milliseconds(),
	}
	if err != nil {
		step.Error = err.Error()
	}
	c.append(step)
}

func (c *Collector) RecordEvaluator(evaluator, phase string, parseOK bool, details map[string]interface{}) {
	ok := parseOK
	step := Step{
		Kind:    StepEvaluator,
		Module:  evaluator,
		Phase:   phase,
		OK:      &ok,
		Details: details,
	}
	c.append(step)
}

func (c *Collector) RecordAlignment(phase string, issues []string) {
	c.append(Step{
		Kind:    StepAlignment,
		Phase:   phase,
		Message: joinIssues(issues),
		Details: map[string]interface{}{"issues": issues},
	})
}

func (c *Collector) RecordWarning(module, message string) {
	c.append(Step{Kind: StepWarning, Module: module, Message: message})
}

func (c *Collector) RecordHealing(fromScore, toScore float64, message string) {
	c.append(Step{
		Kind:    StepHealingRetry,
		Message: message,
		Details: map[string]interface{}{
			"previous_score": fromScore,
			"attempt_score":  toScore,
		},
	})
}

func joinIssues(issues []string) string {
	if len(issues) == 0 {
		return ""
	}
	if len(issues) == 1 {
		return issues[0]
	}
	out := issues[0]
	for i := 1; i < len(issues); i++ {
		out += "; " + issues[i]
	}
	return out
}
