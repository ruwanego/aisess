package handoff

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"aisess/internal/sources"
)

func RunHandoff(args []string) {
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
	fs.Parse(args)

	if *to == "" {
		fmt.Fprintln(os.Stderr, "handoff requires --to codex|claude|antigravity")
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
