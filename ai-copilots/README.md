# strop ai-copilots

Operator / author agent pack for strop: pipeline layout, orchestration loops, and human review.

Canonical source lives here. Wiring is done by the AI copilot when you ask it to execute [BOOTSTRAP.md](BOOTSTRAP.md).

**Minimal prompt:**

> Wire strop ai-copilots using BOOTSTRAP.md

**Example (Cursor, dependency or nested checkout):**

> Execute `ai-copilots/BOOTSTRAP.md` in wire mode. Cursor on macOS. Resolve module with `go list -m -f '{{.Dir}}' github.com/behaviorengineering/strop`.

Skills index: [skills/README.md](skills/README.md). Entry for agents: [../AGENTS.md](../AGENTS.md).
