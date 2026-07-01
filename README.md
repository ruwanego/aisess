# aisess

Go CLI to list, view, and hand off local session history between Claude
Code, Codex CLI, and Antigravity — so switching tools mid-task when one
hits a usage limit doesn't mean starting over.

Only two external calls exist in the whole tool: HTTP to OpenRouter (for
`handoff`'s optional narrative step) and reading files on disk. Everything
else is the Go standard library.

## Build

```
go build -o aisess ./cmd/aisess
```

## Usage

```
aisess list    [--tool claude|codex|antigravity|all] [--project SUBSTR] [--n N] [--paths]
aisess show    <index|path> [--tool ...] [--project ...] [--raw]
aisess grep    <term> [--tool ...] [--project ...] [--n N]
aisess handoff <index|path> --to codex|claude|antigravity [--out stdout|file|both] [--no-llm]
```

```
aisess list                       # most recent 20 sessions across all tools
aisess list --tool codex --n 50
aisess show 3                     # 3rd row from a matching `list`
aisess show ~/.claude/projects/-home-you-repo/sessions/abc123.jsonl
aisess grep "confinement theorem" --tool claude
aisess handoff 1 --tool claude --to codex --out both
```

`show <index>`/`handoff <index>` re-resolve the same discovery + filters
`list` used, so pass matching `--tool`/`--project` flags if you used them.
Flags can go before or after the positional index/path — either order works.

## Where each tool's sessions live

| Tool         | Location                                                                 |
|--------------|---------------------------------------------------------------------------|
| Claude Code  | `~/.claude/projects/<encoded-path>/[sessions/]<uuid>.jsonl`               |
| Codex CLI    | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` (or `$CODEX_HOME`)         |
| Antigravity  | `~/.gemini/antigravity[-cli]/brain/<conv-id>/.system_generated/logs/*.jsonl` |

## Switching agents when you hit a limit: `handoff`

The point of `handoff` is to let you jump from one tool to another mid-task
without re-explaining everything, and without burning a pile of tokens (or
re-triggering the same limit) by dumping the raw transcript into the new
tool. It works in two stages:

**1. Deterministic extraction (Go, offline, free).** `internal/sources/todo.go`
reads tool-call JSON directly (not the flattened display text) and pulls
out:
- the most recent TODO/plan state (Claude Code's `TodoWrite`, Codex's
  `update_plan`-shaped tools, and similar are matched by pattern, not a
  hardcoded schema) — and whether the session ended **mid-list** (an
  `in_progress` item, or a mix of completed/pending). This fact is never
  handed to an LLM to determine — it's a pure function of the extracted
  data, because getting it wrong is the one mistake that actually costs you
  work.
- every file path seen in tool-call inputs, deduplicated.
- the last N turns (`--turns`, default 8), trimmed.

This compact digest — not the transcript — is the only thing that ever
leaves your machine.

**2. Optional narrative synthesis (OpenRouter).** If an API key is found, the
digest is sent to a small model (`--model`, defaults to `OPENROUTER_MODEL` env
or `openai/gpt-4o-mini`) with a system prompt that restricts it to four fields:
`task_summary`, `key_decisions`, `next_step`, `warnings`. It's explicitly told
not to restate or invent file paths or TODO state — those are rendered from the
digest directly. The response is parsed as strict JSON first, then a
brace-matched extraction as a fallback, then — if neither works — the raw text is
shown but clearly labeled `(model response wasn't valid structured output)`
rather than silently trusted. `--no-llm` skips this stage entirely (zero tokens
spent); without an API key it's skipped automatically with a warning on stderr.
Either way you still get the full ground-truth document.

**Delivery (`--out`):**
- `stdout` (default) — pipe or copy it yourself.
- `file` — writes into whichever file the target tool auto-loads at session
  start: `CLAUDE.md` for Claude Code, `AGENTS.md` for Codex CLI. A
  `<!-- aisess-handoff:start/end -->` marker pair means re-running replaces
  only that block, leaving any hand-written instructions in the file alone.
  Antigravity has no confirmed hook for an arbitrary root file being
  auto-loaded into a *new* conversation, so its handoff goes to a
  standalone `AISESS_HANDOFF.md` — you paste it as your first message.
- `both` — does both of the above.

`OPENROUTER_BASE_URL` overrides the API base, useful for a self-hosted
OpenAI-compatible gateway or for testing against a mock server.

**Note on API keys:** The API key is read from a file, not an environment
variable. Set `OPENROUTER_API_KEY_FILE` to specify a custom path, or place it
at `~/.openrouter_apikey` (default). File-based storage keeps credentials out
of shell history and `ps` output.

## Honesty about format coverage

Claude Code's JSONL schema (`message.role` / `message.content` blocks of
`text` / `tool_use` / `tool_result`) is well-documented and parsed
precisely, including its `TodoWrite` tool for TODO/plan extraction.

Codex's rollout format and Antigravity's log format are **not** fully
publicly specified and have changed across versions (Codex's own tooling
distinguishes "new", "mid", and "oldest" schema variants). For those two,
`internal/sources/generic.go` does best-effort extraction: it looks for
`role`/`type` fields and walks `content` for `text`/`tool_use`/`tool_result`
shapes, falling back to a compact JSON dump rather than silently dropping
anything. TODO extraction (`internal/sources/todo.go`) is similarly
pattern-based (any tool call whose name contains "todo"/"plan"/"task" and
whose input has an array of `{text-like field, status-like field}` items)
rather than tied to one exact schema. If a line renders with a `?` role or
a TODO list isn't picked up on your machine, run `aisess show <path> --raw`
to see the decoded JSON for that line and adjust the relevant extractor —
they're small, single-purpose functions.

Also note: the VS Code-based Antigravity IDE has, at times, stored chat
history in a SQLite `globalStorage` database instead of the JSONL brain logs
this tool reads. If `list --tool antigravity` comes back empty but you know
you've used Antigravity, check
`~/Library/Application Support/Antigravity/User/globalStorage/` (macOS) or
the platform equivalent by hand — reading SQLite would need an extra
dependency, which was intentionally left out to keep this `go build`-only.

## Design notes

- Pure standard library except the `handoff` command's OpenRouter HTTP
  call — builds offline, no `go.sum` drift.
- `Session` metadata (path, mtime, project) is separated from `Session.Load()`
  (parses the file), so `list` stays cheap even with hundreds of sessions.
- Everything funnels through a normalized `Message{Role, Text, ToolName,
  Timestamp, Raw}` regardless of source tool. Display logic reads `Text`;
  extraction logic that needs structured tool-call input (TODO state, files
  touched) reads `Raw` directly rather than re-parsing the flattened
  display text.
- `handoff`'s ground-truth/narrative split is intentional: anything an LLM
  could hallucinate about (file paths, TODO status) is computed in Go and
  only ever echoed to the model for context, never regenerated from its
  output.
