package handoff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultBaseURL = "https://openrouter.ai/api/v1/chat/completions"

// systemPrompt is deliberately restrictive: the model only gets the
// compact Digest (never the raw transcript) and is told exactly which
// fields it's allowed to produce. Ground-truth fields (files_touched,
// todo_state) are echoed to it for context but it's explicitly told not to
// restate or alter them — those are rendered from the Digest directly by
// Go code, not from anything the model says.
const systemPrompt = `You compress an AI coding-agent session digest into a short handoff brief for a DIFFERENT coding tool that will continue the same task after a usage limit was hit.

You receive a pre-extracted JSON digest, not the raw transcript. Treat every field in it as verified ground truth.

Write ONLY these fields, nothing else:
- task_summary: at most 2 plain sentences on what is being worked on right now.
- key_decisions: up to 5 short bullets on decisions/constraints established mid-session that a fresh tool wouldn't otherwise know. Omit if none are evident.
- next_step: one concrete sentence: the single most useful next action.
- warnings: up to 3 short bullets flagging anything risky about resuming (e.g. an in-progress TODO item, an ambiguous last message). Omit if none.

Do NOT invent file paths. Do NOT restate or re-derive files_touched or todo_state — the caller already has those verbatim and will render them separately. Do NOT guess at facts not present in the digest; say "unclear from digest" instead.

Respond with ONLY a single JSON object matching:
{"task_summary": "...", "key_decisions": ["..."], "next_step": "...", "warnings": ["..."]}
No prose outside the JSON. Target well under 200 words total.`

const (
	startMarker = "<!-- aisess-handoff:start -->"
	endMarker   = "<!-- aisess-handoff:end -->"
)

type turnPayload struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type promptPayload struct {
	Tool         string        `json:"tool"`
	Project      string        `json:"project,omitempty"`
	MessageCount int           `json:"message_count"`
	EndedMidTodo bool          `json:"ended_mid_todo"`
	FilesTouched []string      `json:"files_touched,omitempty"`
	LastTurns    []turnPayload `json:"last_turns"`
}

// Narrative holds the model-synthesized fields. Everything else in the
// final handoff document comes from the Digest, not from here.
type Narrative struct {
	TaskSummary   string   `json:"task_summary"`
	KeyDecisions  []string `json:"key_decisions"`
	NextStep      string   `json:"next_step"`
	Warnings      []string `json:"warnings"`
	ParseFallback bool     `json:"-"` // true if the response wasn't valid JSON and TaskSummary holds raw text instead
	ModelUsed     string   `json:"-"`
	PromptBytes   int      `json:"-"` // size of what was actually sent, for token-savings visibility
}

// Client talks to OpenRouter's OpenAI-compatible chat completions endpoint.
// BaseURL is overridable via OPENROUTER_BASE_URL, which also makes this
// usable against OpenAI-compatible gateways/mocks.
type Client struct {
	APIKey     string
	BaseURL    string
	Model      string
	MaxTokens  int
	HTTPClient *http.Client
}

// NewClientFromEnv builds a Client using OPENROUTER_API_KEY_FILE /
// OPENROUTER_BASE_URL from the environment. Available() should be checked
// before calling Summarize.
func NewClientFromEnv(model string, maxTokens int) *Client {
	base := os.Getenv("OPENROUTER_BASE_URL")
	if base == "" {
		base = defaultBaseURL
	}

	apiKey := readAPIKeyFromFile()

	return &Client{
		APIKey:     apiKey,
		BaseURL:    base,
		Model:      model,
		MaxTokens:  maxTokens,
		HTTPClient: &http.Client{Timeout: 45 * time.Second},
	}
}

// readAPIKeyFromFile reads the OpenRouter API key from a file.
// Checks OPENROUTER_API_KEY_FILE env var, then ~/.openrouter_apikey.
func readAPIKeyFromFile() string {
	var keyPath string

	// Check for explicit env var first
	if keyPath = os.Getenv("OPENROUTER_API_KEY_FILE"); keyPath == "" {
		// Fall back to default location
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		keyPath = home + "/.openrouter_apikey"
	}

	data, err := os.ReadFile(keyPath)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

// Available reports whether an API key is configured. Callers should treat
// its absence as "skip the LLM step", not as an error — the digest alone
// is still a useful handoff document.
func (c *Client) Available() bool { return c != nil && c.APIKey != "" }

type chatRequest struct {
	Model       string        `json:"model"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
	Messages    []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Summarize sends the digest (and nothing else) to the configured model
// and returns the narrative fields it produced.
func (c *Client) Summarize(d Digest) (*Narrative, error) {
	if !c.Available() {
		return nil, fmt.Errorf("OPENROUTER_API_KEY_FILE not set or ~/.openrouter_apikey not found")
	}

	payload, err := d.PromptJSON()
	if err != nil {
		return nil, fmt.Errorf("encoding digest: %w", err)
	}

	reqBody := chatRequest{
		Model:       c.Model,
		MaxTokens:   c.MaxTokens,
		Temperature: 0.2, // low: this is compression, not creative writing — favors consistency over variety
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: payload},
		},
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.BaseURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Title", "aisess-handoff")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling OpenRouter: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading OpenRouter response: %w", err)
	}

	var cr chatResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return nil, fmt.Errorf("decoding OpenRouter response (status %d): %w", resp.StatusCode, err)
	}
	if cr.Error != nil {
		return nil, fmt.Errorf("OpenRouter error: %s", cr.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenRouter returned status %d", resp.StatusCode)
	}
	if len(cr.Choices) == 0 {
		return nil, fmt.Errorf("OpenRouter returned no choices")
	}

	content := cr.Choices[0].Message.Content
	n := parseNarrative(content)
	n.ModelUsed = cr.Model
	if n.ModelUsed == "" {
		n.ModelUsed = c.Model
	}
	n.PromptBytes = len(payload)
	return n, nil
}

// parseNarrative tries strict JSON first, then a brace-matched extraction
// (models occasionally wrap JSON in prose or code fences despite
// instructions not to), then falls back to treating the raw content as an
// unstructured task summary — explicitly flagged, never silently trusted
// as if it were structured.
func parseNarrative(content string) *Narrative {
	var n Narrative
	trimmed := strings.TrimSpace(content)

	if err := json.Unmarshal([]byte(trimmed), &n); err == nil {
		return &n
	}

	if obj := extractJSONObject(trimmed); obj != "" {
		if err := json.Unmarshal([]byte(obj), &n); err == nil {
			return &n
		}
	}

	return &Narrative{TaskSummary: trimmed, ParseFallback: true}
}

// extractJSONObject finds the first balanced {...} span in s, respecting
// string literals so braces inside quoted text don't confuse the count.
func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}
