---
name: inference-pace
description: >-
  Batch LLM inference pacing: one admit gate per job context, AIMD on throttle
  signals, core.LLM decorator, inspect via start log and CurrentRPS. Use when
  wiring digest-scale jobs, 429/breaker storms, or transient_infra mechanical
  recovery (not PlanRepair).
---

# Inference pace

**Module:** `github.com/behaviorengineering/strop` (orchestration failure class `transient_infra`).

**Related:** `.cursor/skills/strop-orchestration/SKILL.md` (§ pipeline trajectory / `transient_infra`).

---

## When to load

- Adding or reviewing batch LLM jobs (composition loops, RLM hops, judge CoT generators).
- Provider returns 429, rate limit, or circuit breaker errors under parallel load.
- Classifying failures: mechanical pace vs semantic `PlanRepair`.

---

## Pattern (product-neutral)

| Piece | Role |
| --- | --- |
| **Gate** | One token bucket per batch job, attached on `context.Context` at job start |
| **Decorator** | Wrap `core.LLM` so `Generate` / `GenerateWithContent` call `gate.Wait` before the inner LLM |
| **AIMD** | On throttle-like errors only: multiplicative decrease + short cooldown; on success streak: additive increase between min and max RPS |
| **Unlimited** | `max_rps <= 0` disables pacing; retries MUST fail fast on throttle (no retry storm) |
| **Inspect** | Job start log: max/min RPS and mode; tests and ops may read `CurrentRPS()` on the gate |

**CONSTRAINT:** Batch jobs that share one provider budget MUST share one gate on the same ctx for the run.

**CONSTRAINT:** `transient_infra` (429, breaker, rate limit) MUST use mechanical pace/backoff only. MUST NOT call `PlanRepair` or semantic compensators for those errors.

Enforcement: trace `Generate` path from job entry; confirm one gate on ctx and decorator on every LLM create path (including judge/factory seams).

Violation: STOP, wire `WrapLLM` or equivalent after `CreateLLM`; do not add unbounded retries on 429.

CORRECT:
```text
ON job start: attach gate to ctx (max_rps > 0)
ON each CreateLLM: wrap inner LLM → Wait before Generate
ON throttle error: RecordThrottle + cooldown; optional single log line with new RPS
```

PROHIBITED:
```text
IF 429 → PlanRepair composition strategy
Parallel workers each with their own uncapped gate
Retry Generate in a tight loop without pace on breaker open
```

---

## Host wiring (conceptual)

Hosts implement the gate and `wrapDigestLLM`-style decorator in application code. Libraries document the pattern here; strop does not require a specific env var name in this skill.

Interactive CLIs and one-shot review commands MAY leave `max_rps <= 0` unless the operator opts in.

---

## Review checklist (binary)

- [ ] One gate per batch job ctx when pacing is enabled
- [ ] Every LLM path used in that job uses the pace decorator (RLM **and** factory/judge `CreateLLM`)
- [ ] Throttle errors decrease admit rate or honor cooldown (when adaptive)
- [ ] Unlimited mode does not retry throttle errors aggressively
- [ ] `transient_infra` handling does not invoke semantic repair for 429/breaker
