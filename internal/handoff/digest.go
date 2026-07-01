package handoff

import (
	"encoding/json"

	"aisess/internal/sources"
)

// Digest is the compact, ground-truth extraction from one session.
type Digest struct {
	Tool         sources.Tool
	SessionID    string
	Project      string
	Path         string
	MessageCount int

	TodoState    *sources.TodoState
	EndedMidTodo bool
	FilesTouched []string

	LastTurns []sources.Message
}

// Build extracts a Digest from a fully-loaded session.
func Build(sess *sources.Session, msgs []sources.Message, turns int) Digest {
	d := Digest{
		Tool:         sess.Tool,
		SessionID:    sess.ID,
		Project:      sess.Project,
		Path:         sess.Path,
		MessageCount: len(msgs),
	}

	d.TodoState = sources.ExtractTodoState(msgs)
	d.EndedMidTodo = d.TodoState.EndedMidTodo()
	d.FilesTouched = sources.ExtractFilesTouched(msgs)

	start := 0
	if turns > 0 && len(msgs) > turns {
		start = len(msgs) - turns
	}
	for _, m := range msgs[start:] {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		d.LastTurns = append(d.LastTurns, sources.Message{
			Role:      m.Role,
			Text:      trimText(m.Text, 600),
			Timestamp: m.Timestamp,
		})
	}
	return d
}

func trimText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// PromptJSON renders the digest as the compact JSON payload sent to the LLM.
func (d Digest) PromptJSON() (string, error) {
	p := promptPayload{
		Tool:         string(d.Tool),
		Project:      d.Project,
		MessageCount: d.MessageCount,
		EndedMidTodo: d.EndedMidTodo,
		FilesTouched: d.FilesTouched,
	}
	for _, m := range d.LastTurns {
		p.LastTurns = append(p.LastTurns, turnPayload{Role: m.Role, Text: m.Text})
	}
	b, err := json.Marshal(p)
	return string(b), err
}
