package sources

import (
	"time"
)

// Tool represents a coding AI tool
type Tool string

const (
	ToolClaude      Tool = "claude"
	ToolCodex       Tool = "codex"
	ToolAntigravity Tool = "antigravity"
)

// Session represents a single session from a tool
type Session struct {
	Tool    Tool
	ID      string
	Path    string
	Project string
	ModTime time.Time
}

// Load parses and returns the messages from the session file
func (s *Session) Load() ([]Message, error) {
	// TODO: implement session loading
	return []Message{}, nil
}

// Message represents a single message in a session
type Message struct {
	Role      string
	Text      string
	ToolName  string
	Timestamp time.Time
	Raw       map[string]interface{}
}

// TodoState represents the extracted TODO/plan state
type TodoState struct {
	Items     []TodoItem
	Source    string
	Timestamp time.Time
}

// TodoItem is one entry in a plan/todo list
type TodoItem struct {
	Text   string
	Status string // "pending", "in_progress", "completed"
}

// EndedMidTodo reports whether the session ended mid-todo
func (t *TodoState) EndedMidTodo() bool {
	if t == nil {
		return false
	}
	hasCompleted, hasPending := false, false
	for _, it := range t.Items {
		switch it.Status {
		case "in_progress":
			return true
		case "completed":
			hasCompleted = true
		case "pending":
			hasPending = true
		}
	}
	return hasCompleted && hasPending
}

// DiscoverAll discovers all sessions for the given tools
func DiscoverAll(tools []Tool) ([]Session, error) {
	// TODO: implement session discovery
	return []Session{}, nil
}

// FirstNonEmpty returns the first non-empty text from messages
func FirstNonEmpty(msgs []Message) string {
	for _, m := range msgs {
		if m.Text != "" {
			return m.Text
		}
	}
	return ""
}

// ExtractTodoState extracts TODO state from messages
func ExtractTodoState(msgs []Message) *TodoState {
	return ExtractTodoStateFromMessages(msgs)
}

// ExtractFilesTouched extracts file paths from messages
func ExtractFilesTouched(msgs []Message) []string {
	return ExtractFilesTouchedFromMessages(msgs)
}

// firstString returns the first non-empty string value for the given keys
func firstString(m map[string]interface{}, keys ...string) (string, bool) {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v, true
		}
	}
	return "", false
}
