# BOOTSTRAP — strop ai-copilots

**Audience:** Any AI agent (Cursor, GitHub Copilot, Claude Code, Codex) in a workspace that depends on or checks out this module.

**Goal:** Wire host IDE discovery to canonical content under `ai-copilots/`. **MUST NOT** copy skill bodies unless symlinks or junctions fail and the user approves copy fallback.

**Module path:** `github.com/behaviorengineering/strop`

---

## When to run

| Mode | Phases |
|------|--------|
| **Wire only** | 0 → 2 → 3 → 4 |
| **Refresh content + wire** | 0 → 1 → 2 → 3 → 4 |

---

## Phase 0 — Resolve module root

```bash
MOD="$(go list -m -f '{{.Dir}}' github.com/behaviorengineering/strop)"
test -d "$MOD/ai-copilots/skills" || { echo "missing ai-copilots under $MOD"; exit 1; }
echo "Module Dir: $MOD"
```

If `go list` is unavailable, use a known nested checkout path only when it clearly contains `ai-copilots/`. MUST NOT invent a path. Re-run wire after module version bumps.

---

## Phase 1 — Refresh content (optional)

Edit only files under `$MOD/ai-copilots/`.

```text
ai-copilots/
  README.md
  BOOTSTRAP.md
  skills/
    README.md
    strop-pipeline-pattern/SKILL.md
    strop-orchestration/SKILL.md
    strop-human-review/SKILL.md
```

---

## Phase 2 — Ask IDE and OS if unknown

1. IDE: Cursor, GitHub Copilot, Claude Code, Codex
2. OS: macOS/Linux symlink vs Windows junction/copy
3. Workspace: strop alone vs nested vs dependency-only

---

## Phase 3 — Wire discovery

| Host skill name | Path under `$MOD` |
|-----------------|-------------------|
| `strop-pipeline-pattern` | `ai-copilots/skills/strop-pipeline-pattern/` |
| `strop-orchestration` | `ai-copilots/skills/strop-orchestration/` |
| `strop-human-review` | `ai-copilots/skills/strop-human-review/` |

| IDE | Skills |
|-----|--------|
| Cursor | `.cursor/skills/**/SKILL.md` |
| GitHub Copilot | `.github/skills/**/SKILL.md` |
| Claude Code | `.claude/skills/**/SKILL.md` |
| Codex | `.codex/skills/**/SKILL.md` |

**Cursor (macOS/Linux)** from host workspace root:

```bash
MOD="$(go list -m -f '{{.Dir}}' github.com/behaviorengineering/strop)"
mkdir -p .cursor/skills
ln -snf "$MOD/ai-copilots/skills/strop-pipeline-pattern" .cursor/skills/strop-pipeline-pattern
ln -snf "$MOD/ai-copilots/skills/strop-orchestration" .cursor/skills/strop-orchestration
ln -snf "$MOD/ai-copilots/skills/strop-human-review" .cursor/skills/strop-human-review
```

When the workspace root is this module:

```bash
mkdir -p .cursor/skills
ln -snf ../ai-copilots/skills/strop-pipeline-pattern .cursor/skills/strop-pipeline-pattern
ln -snf ../ai-copilots/skills/strop-orchestration .cursor/skills/strop-orchestration
ln -snf ../ai-copilots/skills/strop-human-review .cursor/skills/strop-human-review
```

**Windows:** junction or developer-mode symlink; copy only with user approval.

---

## Phase 4 — Verify

```bash
MOD="$(go list -m -f '{{.Dir}}' github.com/behaviorengineering/strop)"
ls -la .cursor/skills/strop-orchestration
test -f .cursor/skills/strop-orchestration/SKILL.md
test -f "$MOD/ai-copilots/BOOTSTRAP.md"
```

Ask before committing host wiring.
