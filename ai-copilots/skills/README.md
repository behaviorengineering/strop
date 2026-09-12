# strop skills

Portable instructions for agents that build on this library. Canonical path is this directory. Hosts symlink the folders below into their skills directory; they MUST NOT treat a copy as source of truth.

| Skill | Load when |
|-------|-----------|
| [strop-pipeline-pattern/SKILL.md](strop-pipeline-pattern/SKILL.md) | Adding a pipeline or job, JobRunner clients/modules, evaluators |
| [strop-orchestration/SKILL.md](strop-orchestration/SKILL.md) | Refinement loops, composition walks, regenerate / max-versions |
| [strop-human-review/SKILL.md](strop-human-review/SKILL.md) | Gate, reviewflow ports, reject-and-regenerate |

Wire discovery: [../BOOTSTRAP.md](../BOOTSTRAP.md). Entry: [../../AGENTS.md](../../AGENTS.md). Human pitch: [../../README.md](../../README.md).

DSPy-Go module wiring and XML structured output stay in the host shared pack (`dspy-*` skills).
