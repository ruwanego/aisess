package sources

import (
	"strings"
	"time"
)

// ToolCall is a normalized view of one tool_use/function_call block
type ToolCall struct {
	Name      string
	Input     map[string]interface{}
	Timestamp time.Time
}

// WalkToolCalls collects every tool_use/function_call block across a session's messages
func WalkToolCalls(msgs []Message) []ToolCall {
	var calls []ToolCall
	for _, m := range msgs {
		if m.Raw == nil {
			continue
		}
		src := m.Raw
		if nested, ok := m.Raw["message"].(map[string]interface{}); ok {
			src = nested
		}
		items, ok := src["content"].([]interface{})
		if !ok {
			continue
		}
		for _, it := range items {
			im, ok := it.(map[string]interface{})
			if !ok {
				continue
			}
			t, _ := im["type"].(string)
			switch t {
			case "tool_use", "function_call", "tool_call":
				name, _ := firstString(im, "name", "tool_name", "function")
				input, _ := im["input"].(map[string]interface{})
				if input == nil {
					input, _ = im["arguments"].(map[string]interface{})
				}
				calls = append(calls, ToolCall{Name: name, Input: input, Timestamp: m.Timestamp})
			}
		}
	}
	return calls
}

var todoToolHints = []string{"todo", "plan", "task"}

func looksLikeTodoTool(name string) bool {
	n := strings.ToLower(name)
	for _, h := range todoToolHints {
		if strings.Contains(n, h) {
			return true
		}
	}
	return false
}

// ExtractTodoStateFromMessages scans tool calls for TODO lists and returns the last one
func ExtractTodoStateFromMessages(msgs []Message) *TodoState {
	var last *TodoState
	for _, c := range WalkToolCalls(msgs) {
		if !looksLikeTodoTool(c.Name) || c.Input == nil {
			continue
		}
		items := findTodoItems(c.Input)
		if len(items) == 0 {
			continue
		}
		last = &TodoState{Items: items, Source: c.Name, Timestamp: c.Timestamp}
	}
	return last
}

func findTodoItems(input map[string]interface{}) []TodoItem {
	for _, v := range input {
		arr, ok := v.([]interface{})
		if !ok || len(arr) == 0 {
			continue
		}
		items := make([]TodoItem, 0, len(arr))
		ok = true
		for _, e := range arr {
			em, isMap := e.(map[string]interface{})
			if !isMap {
				ok = false
				break
			}
			text, hasText := firstString(em, "content", "text", "step", "description", "task")
			if !hasText {
				ok = false
				break
			}
			status, _ := firstString(em, "status", "state")
			items = append(items, TodoItem{Text: text, Status: normalizeTodoStatus(status)})
		}
		if ok && len(items) > 0 {
			return items
		}
	}
	return nil
}

func normalizeTodoStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "done", "complete", "completed", "finished":
		return "completed"
	case "in_progress", "in-progress", "inprogress", "active", "doing", "running":
		return "in_progress"
	default:
		return "pending"
	}
}

// ExtractFilesTouchedFromMessages returns file paths from tool calls
func ExtractFilesTouchedFromMessages(msgs []Message) []string {
	keys := []string{"file_path", "path", "target_file", "filename", "filepath", "file"}
	seen := map[string]bool{}
	var out []string
	for _, c := range WalkToolCalls(msgs) {
		if c.Input == nil {
			continue
		}
		if p, ok := firstString(c.Input, keys...); ok && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}
