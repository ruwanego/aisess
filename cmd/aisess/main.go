// Command aisess lists and views local session history for Claude Code,
// Codex CLI, and Antigravity — three agentic coding tools that each keep
// their own JSONL transcripts on disk. See internal/sources for where each
// tool's files live.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ruwanego/aisess/internal/handoff"
	"github.com/ruwanego/aisess/internal/sources"
)

const (
	colorReset  = "\033[0m"
	colorDim    = "\033[2m"
	colorBold   = "\033[1m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorGray   = "\033[90m"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "list", "ls":
		runList(os.Args[2:])
	case "show", "view", "cat":
		runShow(os.Args[2:])
	case "grep", "search":
		runGrep(os.Args[2:])
	case "handoff":
		runHandoff(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `aisess — browse local Claude Code / Codex CLI / Antigravity session history

Usage:
  aisess list    [--tool claude|codex|antigravity|all] [--project SUBSTR] [--n N] [--paths]
  aisess show    <index|path> [--tool ...] [--project ...] [--raw]
  aisess grep    <term> [--tool ...] [--project ...] [--n N]
  aisess handoff <index|path> --to codex|claude|antigravity [--out stdout|file|both] [--no-llm]

Examples:
  aisess list                          # most recent 20 sessions across all tools
  aisess list --tool codex --n 50
  aisess show 3                        # show the 3rd session from a matching list run
  aisess show ~/.claude/projects/.../abc123.jsonl
  aisess grep "confinement theorem" --tool claude
  aisess handoff 1 --to codex --out both
      # extracts TODO state + files touched + last turns from session 1,
      # optionally asks OpenRouter to write task_summary/decisions/next_step,
      # and writes into ./AGENTS.md (Codex auto-loads it) plus prints to stdout.

Environment:
  OPENROUTER_API_KEY   required for the LLM narrative step in handoff; omitted -> ground-truth-only doc
  OPENROUTER_MODEL     default model slug for handoff (falls back to openai/gpt-4o-mini)
  OPENROUTER_BASE_URL  override the API base, e.g. for a self-hosted gateway or testing
  CODEX_HOME           override ~/.codex for Codex CLI discovery
`)
}

// reorderArgs moves all recognized flags (and their values) to the front
// of args, leaving positional arguments at the end. This is needed because
// the standard flag package stops parsing at the first non-flag token, so
// without this, `aisess handoff 1 --to codex` would silently ignore
// --to codex — bad, since these commands take their positional index/path
// alongside several flags and both orderings are natural to type.
func reorderArgs(args []string, boolFlags map[string]bool) []string {
	var flagsOut, posOut []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			posOut = append(posOut, a)
			continue
		}
		name := strings.TrimLeft(a, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			// --flag=value: self-contained, no lookahead needed.
			flagsOut = append(flagsOut, a)
			continue
		}
		flagsOut = append(flagsOut, a)
		if boolFlags[name] {
			continue // boolean flag: no separate value token
		}
		if i+1 < len(args) {
			flagsOut = append(flagsOut, args[i+1])
			i++
		}
	}
	return append(flagsOut, posOut...)
}

func parseToolFlag(s string) ([]sources.Tool, error) {
	switch strings.ToLower(s) {
	case "", "all":
		return []sources.Tool{sources.ToolClaude, sources.ToolCodex, sources.ToolAntigravity}, nil
	case "claude":
		return []sources.Tool{sources.ToolClaude}, nil
	case "codex":
		return []sources.Tool{sources.ToolCodex}, nil
	case "antigravity", "ag":
		return []sources.Tool{sources.ToolAntigravity}, nil
	default:
		return nil, fmt.Errorf("unknown --tool %q (want claude, codex, antigravity, or all)", s)
	}
}

func filterSessions(all []sources.Session, projectSubstr string) []sources.Session {
	if projectSubstr == "" {
		return all
	}
	var out []sources.Session
	needle := strings.ToLower(projectSubstr)
	for _, s := range all {
		if strings.Contains(strings.ToLower(s.Project), needle) ||
			strings.Contains(strings.ToLower(s.Path), needle) {
			out = append(out, s)
		}
	}
	return out
}

// --- list ----------------------------------------------------------------

func runList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	tool := fs.String("tool", "all", "claude, codex, antigravity, or all")
	project := fs.String("project", "", "only show sessions whose project/path contains this substring")
	n := fs.Int("n", 20, "max sessions to show (0 = no limit)")
	showPaths := fs.Bool("paths", false, "print full file paths instead of the index table")
	fs.Parse(args)

	tools, err := parseToolFlag(*tool)
	must(err)

	all, err := sources.DiscoverAll(tools)
	must(err)
	all = filterSessions(all, *project)

	if len(all) == 0 {
		fmt.Println("No sessions found. If you expected some, check that the relevant tool has been run on this machine and that its config dir isn't overridden (CLAUDE_CONFIG_DIR / CODEX_HOME).")
		return
	}

	shown := all
	if *n > 0 && len(shown) > *n {
		shown = shown[:*n]
	}

	if *showPaths {
		for _, s := range shown {
			fmt.Println(s.Path)
		}
		return
	}

	fmt.Printf("%s%-4s %-12s %-19s %-28s %s%s\n", colorBold, "#", "TOOL", "MODIFIED", "PROJECT", "PREVIEW", colorReset)
	for i, s := range shown {
		preview := previewFor(&s)
		proj := s.Project
		if proj == "" {
			proj = "-"
		}
		if len(proj) > 26 {
			proj = "…" + proj[len(proj)-25:]
		}
		fmt.Printf("%-4d %s%-12s%s %-19s %-28s %s\n",
			i+1,
			toolColor(s.Tool), string(s.Tool), colorReset,
			s.ModTime.Local().Format("2006-01-02 15:04:05"),
			proj,
			preview,
		)
	}
	if *n > 0 && len(all) > *n {
		fmt.Printf("%s… %d more (raise --n to see them)%s\n", colorDim, len(all)-*n, colorReset)
	}
}

// previewFor loads just enough of a session to show a one-line preview.
// Full parsing of every session in a `list` call is intentionally cheap:
// files are typically small, and this keeps the tool dependency-free.
func previewFor(s *sources.Session) string {
	msgs, err := s.Load()
	if err != nil {
		return colorRed + "(unreadable: " + err.Error() + ")" + colorReset
	}
	p := sources.FirstNonEmpty(msgs)
	if p == "" {
		return colorDim + fmt.Sprintf("(%d messages, no preview text)", len(msgs)) + colorReset
	}
	return p
}

// --- show ------------------------------------------------------------------

func runShow(args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	tool := fs.String("tool", "all", "claude, codex, antigravity, or all (used when resolving an index)")
	project := fs.String("project", "", "filter used when resolving an index (must match what `list` used)")
	raw := fs.Bool("raw", false, "print the raw decoded JSON for each line instead of a formatted transcript")
	fs.Parse(reorderArgs(args, map[string]bool{"raw": true}))

	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "usage: aisess show <index|path> [--tool ...] [--project ...] [--raw]")
		os.Exit(1)
	}

	sess, err := resolveSessionArg(rest[0], *tool, *project)
	must(err)

	msgs, err := sess.Load()
	must(err)

	fmt.Printf("%s%s session%s  %s\n", colorBold, strings.Title(string(sess.Tool)), colorReset, sess.Path)
	if sess.Project != "" {
		fmt.Printf("%sproject:%s %s\n", colorDim, colorReset, sess.Project)
	}
	fmt.Printf("%s%d messages%s\n\n", colorDim, len(msgs), colorReset)

	for _, m := range msgs {
		printMessage(m, *raw)
	}
}

// sessionFromPath lets `show` accept a raw file (or, for Antigravity, log
// directory) path directly, without needing an index from `list`.
func sessionFromPath(path string) sources.Session {
	tool := sources.ToolClaude
	switch {
	case strings.Contains(path, string(os.PathSeparator)+".codex"+string(os.PathSeparator)),
		strings.HasPrefix(strings.ToLower(path), "rollout-"):
		tool = sources.ToolCodex
	case strings.Contains(path, ".gemini"):
		tool = sources.ToolAntigravity
	case strings.Contains(path, ".claude"):
		tool = sources.ToolClaude
	}
	return sources.Session{Tool: tool, Path: path}
}

// resolveSessionArg turns a `list`-style index or a raw file path into a
// Session. Index resolution re-runs discovery with the same --tool/
// --project filters the caller passed, so it must match what `list` used
// to produce the index the person is referencing.
func resolveSessionArg(arg, toolStr, projectStr string) (sources.Session, error) {
	if idx, err := strconv.Atoi(arg); err == nil {
		tools, err := parseToolFlag(toolStr)
		if err != nil {
			return sources.Session{}, err
		}
		all, err := sources.DiscoverAll(tools)
		if err != nil {
			return sources.Session{}, err
		}
		all = filterSessions(all, projectStr)
		if idx < 1 || idx > len(all) {
			return sources.Session{}, fmt.Errorf("index %d out of range (1..%d) — run `aisess list` with matching --tool/--project first", idx, len(all))
		}
		return all[idx-1], nil
	}
	return sessionFromPath(arg), nil
}

func printMessage(m sources.Message, raw bool) {
	if raw {
		fmt.Printf("%+v\n\n", m.Raw)
		return
	}

	label, color := roleLabel(m)
	ts := ""
	if !m.Timestamp.IsZero() {
		ts = colorDim + m.Timestamp.Local().Format("15:04:05") + colorReset + "  "
	}
	fmt.Printf("%s%s%-10s%s %s\n", ts, color, label, colorReset, strings.TrimSpace(m.Text))
	fmt.Println()
}

func roleLabel(m sources.Message) (string, string) {
	switch m.Role {
	case "user":
		return "user", colorCyan
	case "assistant":
		return "assistant", colorGreen
	case "tool":
		label := "tool"
		if m.ToolName != "" {
			label = m.ToolName
		}
		return label, colorYellow
	case "system":
		return "system", colorGray
	default:
		return "?", colorGray
	}
}

// --- grep ------------------------------------------------------------------

func runGrep(args []string) {
	fs := flag.NewFlagSet("grep", flag.ExitOnError)
	tool := fs.String("tool", "all", "claude, codex, antigravity, or all")
	project := fs.String("project", "", "only search sessions whose project/path contains this substring")
	n := fs.Int("n", 200, "max sessions to search (0 = no limit; can be slow across large histories)")
	ignoreCase := fs.Bool("i", true, "case-insensitive match")
	fs.Parse(reorderArgs(args, map[string]bool{"i": true}))

	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "usage: aisess grep <term> [--tool ...] [--project ...] [--n N]")
		os.Exit(1)
	}
	term := rest[0]
	needle := term
	if *ignoreCase {
		needle = strings.ToLower(term)
	}

	tools, err := parseToolFlag(*tool)
	must(err)
	all, err := sources.DiscoverAll(tools)
	must(err)
	all = filterSessions(all, *project)
	if *n > 0 && len(all) > *n {
		all = all[:*n]
	}

	matches := 0
	for _, s := range all {
		msgs, err := s.Load()
		if err != nil {
			continue
		}
		var hitLines []string
		for _, m := range msgs {
			hay := m.Text
			if *ignoreCase {
				hay = strings.ToLower(hay)
			}
			if strings.Contains(hay, needle) {
				hitLines = append(hitLines, fmt.Sprintf("  %s%-10s%s %s", colorForRole(m.Role), m.Role, colorReset, snippetAround(m.Text, term)))
			}
		}
		if len(hitLines) == 0 {
			continue
		}
		matches++
		fmt.Printf("%s%s%s  %s%s%s\n", colorBold, s.Path, colorReset, colorDim, s.ModTime.Local().Format(time.RFC3339), colorReset)
		for _, l := range hitLines {
			fmt.Println(l)
		}
		fmt.Println()
	}
	if matches == 0 {
		fmt.Println("no matches")
	}
}

// --- handoff ---------------------------------------------------------------

func runHandoff(args []string) {
	fs := flag.NewFlagSet("handoff", flag.ExitOnError)
	tool := fs.String("tool", "all", "claude, codex, antigravity, or all (used when resolving an index)")
	project := fs.String("project", "", "filter used when resolving an index (must match what `list` used)")
	to := fs.String("to", "", "required: codex, claude, or antigravity — which tool you're switching to")
	out := fs.String("out", "stdout", "stdout, file, or both")
	dir := fs.String("dir", ".", "directory to write the target file into (with --out file/both)")
	turns := fs.Int("turns", 8, "trailing user/assistant messages to keep verbatim in the digest")
	model := fs.String("model", envOr("OPENROUTER_MODEL", "openai/gpt-4o-mini"), "OpenRouter model slug for the narrative step")
	maxTokens := fs.Int("max-tokens", 400, "cap on LLM output tokens (keeps the narrative terse and keeps cost down)")
	noLLM := fs.Bool("no-llm", false, "skip the OpenRouter call entirely; emit the ground-truth digest only")
	fs.Parse(reorderArgs(args, map[string]bool{"no-llm": true}))

	if *to == "" {
		fmt.Fprintln(os.Stderr, "handoff requires --to codex|claude|antigravity")
		os.Exit(1)
	}
	toTools, err := parseToolFlag(*to)
	must(err)
	if len(toTools) != 1 {
		fmt.Fprintln(os.Stderr, "--to must name exactly one tool: codex, claude, or antigravity")
		os.Exit(1)
	}
	toTool := toTools[0]

	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "usage: aisess handoff <index|path> --to codex|claude|antigravity [--out stdout|file|both] [--no-llm]")
		os.Exit(1)
	}

	sess, err := resolveSessionArg(rest[0], *tool, *project)
	must(err)

	msgs, err := sess.Load()
	must(err)
	if len(msgs) == 0 {
		fmt.Fprintln(os.Stderr, "session has no parseable messages — nothing to hand off")
		os.Exit(1)
	}

	digest := handoff.Build(&sess, msgs, *turns)

	var narrative *handoff.Narrative
	if *noLLM {
		fmt.Fprintln(os.Stderr, "--no-llm set: skipping OpenRouter, emitting ground-truth digest only")
	} else {
		client := handoff.NewClientFromEnv(*model, *maxTokens)
		if !client.Available() {
			fmt.Fprintln(os.Stderr, "OpenRouter API key not found: skipping the LLM narrative step, emitting ground-truth digest only")
		} else {
			n, err := client.Summarize(digest)
			if err != nil {
				fmt.Fprintf(os.Stderr, "OpenRouter call failed (%v) — falling back to ground-truth digest only\n", err)
			} else {
				narrative = n
				fmt.Fprintf(os.Stderr, "%s%d messages -> %d-byte digest sent to %s (todo mid-run: %v)%s\n",
					colorDim, len(msgs), n.PromptBytes, n.ModelUsed, digest.EndedMidTodo, colorReset)
			}
		}
	}

	doc := handoff.Render(digest, narrative, toTool)

	switch *out {
	case "stdout":
		fmt.Println(doc)
	case "file":
		writeHandoffFile(toTool, *dir, doc)
	case "both":
		fmt.Println(doc)
		writeHandoffFile(toTool, *dir, doc)
	default:
		fmt.Fprintf(os.Stderr, "unknown --out %q (want stdout, file, or both)\n", *out)
		os.Exit(1)
	}
}

func writeHandoffFile(toTool sources.Tool, dir, doc string) {
	path, err := handoff.WriteToTarget(toTool, dir, doc)
	must(err)
	_, autoLoaded := handoff.TargetFile(toTool)
	if autoLoaded {
		fmt.Fprintf(os.Stderr, "%swrote %s — %s auto-loads this at session start%s\n", colorGreen, path, strings.Title(string(toTool)), colorReset)
	} else {
		fmt.Fprintf(os.Stderr, "%swrote %s — no confirmed auto-load hook for %s; paste this as your first message instead%s\n", colorYellow, path, strings.Title(string(toTool)), colorReset)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func toolColor(t sources.Tool) string {
	switch t {
	case sources.ToolClaude:
		return colorCyan
	case sources.ToolCodex:
		return colorGreen
	case sources.ToolAntigravity:
		return colorYellow
	default:
		return colorReset
	}
}

func colorForRole(role string) string {
	switch role {
	case "user":
		return colorCyan
	case "assistant":
		return colorGreen
	case "tool":
		return colorYellow
	default:
		return colorGray
	}
}

func snippetAround(text, term string) string {
	lowerText := strings.ToLower(text)
	lowerTerm := strings.ToLower(term)
	idx := strings.Index(lowerText, lowerTerm)
	if idx < 0 {
		return oneLineLocal(text)
	}
	start := idx - 40
	if start < 0 {
		start = 0
	}
	end := idx + len(term) + 40
	if end > len(text) {
		end = len(text)
	}
	prefix := ""
	if start > 0 {
		prefix = "…"
	}
	suffix := ""
	if end < len(text) {
		suffix = "…"
	}
	return prefix + oneLineLocal(text[start:end]) + suffix
}

func oneLineLocal(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.Join(strings.Fields(s), " ")
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
