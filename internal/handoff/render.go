package handoff

import (
	"fmt"
	"strings"
	"time"

	"aisess/internal/sources"
)

// Render produces the final handoff markdown. n may be nil (no LLM was
// used, either because no API key was configured or --no-llm was passed) —
// in that case the doc still contains the full ground-truth digest, just
// without synthesized prose, and says so explicitly.
func Render(d Digest, n *Narrative, to sources.Tool) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## Handoff: %s → %s\n\n", strings.Title(string(d.Tool)), strings.Title(string(to)))
	fmt.Fprintf(&b, "_Generated %s from a %s session (%d messages%s)._\n\n",
		time.Now().Local().Format("2006-01-02 15:04"), d.Tool, d.MessageCount, projectSuffix(d.Project))

	if d.EndedMidTodo {
		fmt.Fprint(&b, "> ⚠️ **This session ended mid-TODO-list.** Verify state below before continuing — do not assume the in-progress item is actually done.\n\n")
	}

	fmt.Fprint(&b, "### Current task\n")
	if n != nil && n.TaskSummary != "" {
		fmt.Fprintln(&b, n.TaskSummary)
		if n.ParseFallback {
			fmt.Fprint(&b, "\n_(model response wasn't valid structured output — showing raw text)_\n")
		}
	} else if last := lastUserText(d.LastTurns); last != "" {
		fmt.Fprintf(&b, "_(no LLM summary — last user message, verbatim)_\n\n%s\n", last)
	} else {
		fmt.Fprint(&b, "_(no summary available)_\n")
	}
	b.WriteString("\n")

	if d.TodoState != nil && len(d.TodoState.Items) > 0 {
		fmt.Fprintf(&b, "### TODO state (from %s, ground truth)\n", d.TodoState.Source)
		for _, it := range d.TodoState.Items {
			mark := " "
			switch it.Status {
			case "completed":
				mark = "x"
			case "in_progress":
				mark = "~"
			}
			fmt.Fprintf(&b, "- [%s] %s (%s)\n", mark, it.Text, it.Status)
		}
		b.WriteString("\n")
	}

	if n != nil && len(n.KeyDecisions) > 0 {
		fmt.Fprint(&b, "### Key decisions\n")
		for _, dec := range n.KeyDecisions {
			fmt.Fprintf(&b, "- %s\n", dec)
		}
		b.WriteString("\n")
	}

	if n != nil && n.NextStep != "" {
		fmt.Fprintf(&b, "### Suggested next step\n%s\n\n", n.NextStep)
	}

	if n != nil && len(n.Warnings) > 0 {
		fmt.Fprint(&b, "### Warnings\n")
		for _, w := range n.Warnings {
			fmt.Fprintf(&b, "- %s\n", w)
		}
		b.WriteString("\n")
	}

	if len(d.FilesTouched) > 0 {
		fmt.Fprint(&b, "### Files touched (ground truth)\n")
		for _, f := range d.FilesTouched {
			fmt.Fprintf(&b, "- `%s`\n", f)
		}
		b.WriteString("\n")
	}

	if len(d.LastTurns) > 0 {
		fmt.Fprint(&b, "### Last exchanges (verbatim, trimmed)\n")
		for _, m := range d.LastTurns {
			fmt.Fprintf(&b, "**%s:** %s\n\n", m.Role, m.Text)
		}
	}

	fmt.Fprint(&b, "---\n")
	if n != nil {
		fmt.Fprintf(&b, "_Narrative by %s via OpenRouter, %d bytes sent (digest only, not the raw transcript)._\n", n.ModelUsed, n.PromptBytes)
	} else {
		fmt.Fprint(&b, "_No LLM used — ground-truth digest only. Set OPENROUTER_API_KEY_FILE (or drop --no-llm) to add a synthesized summary._\n")
	}
	fmt.Fprintf(&b, "_Source: %s (%s)_\n", d.Path, d.SessionID)

	return b.String()
}

func projectSuffix(project string) string {
	if project == "" {
		return ""
	}
	return ", project " + project
}

func lastUserText(turns []sources.Message) string {
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Role == "user" && turns[i].Text != "" {
			return turns[i].Text
		}
	}
	return ""
}
