# Agents

This module is a Go library for tempering LLM outputs (evaluate, refine, gate). Humans read [README.md](README.md).

**Load these skills before you wire pipelines, orchestration loops, or human review on strop:**

1. [ai-copilots/skills/README.md](ai-copilots/skills/README.md) (index)
2. [ai-copilots/skills/strop-pipeline-pattern/SKILL.md](ai-copilots/skills/strop-pipeline-pattern/SKILL.md) (JobRunner, clients, modules, evaluators)
3. [ai-copilots/skills/strop-orchestration/SKILL.md](ai-copilots/skills/strop-orchestration/SKILL.md) (refinement and composition loops)
4. [ai-copilots/skills/strop-human-review/SKILL.md](ai-copilots/skills/strop-human-review/SKILL.md) (Gate, reviewflow ports)

Cross-product DSPy-Go practice (`dspy-*` skills) lives in the host's shared cursor-packs, not in this module.

## Wire host discovery

Skills ship under [ai-copilots/](ai-copilots/). Execute [ai-copilots/BOOTSTRAP.md](ai-copilots/BOOTSTRAP.md) in **wire mode** to symlink into `.cursor/skills` (or Copilot / Claude / Codex skill dirs).

Resolve the module root when strop is only a Go dependency:

```bash
go list -m -f '{{.Dir}}' github.com/behaviorengineering/strop
```

## Package layout

- Public API lives under `pkg/<domain>` (dspy, orchestration, humanreview, …).
- MUST NOT scatter new public packages at the module root.


MUST keep host links pointing at this module's `ai-copilots/skills/` tree. MUST NOT copy skill bodies into the host unless links fail and the user approves.
