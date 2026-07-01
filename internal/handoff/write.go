package handoff

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ruwanego/aisess/internal/sources"
)

// TargetFile returns which file WriteToTarget will write into for a given
// tool, and whether that file is actually auto-loaded by the tool (as
// opposed to a standalone file the developer has to paste manually).
func TargetFile(to sources.Tool) (filename string, autoLoaded bool) {
	switch to {
	case sources.ToolClaude:
		return "CLAUDE.md", true
	case sources.ToolCodex:
		return "AGENTS.md", true
	case sources.ToolAntigravity:
		// Antigravity's own artifacts (implementation_plan.md/task.md) are
		// generated per-conversation by the tool itself; there's no
		// confirmed hook for an arbitrary root file being auto-loaded into
		// a *new* conversation, so this is written standalone and the
		// caller should tell the user to paste it in as their first
		// message.
		return "AISESS_HANDOFF.md", false
	default:
		return "AISESS_HANDOFF.md", false
	}
}

// WriteToTarget writes content into the target tool's file under dir,
// replacing a previous aisess-handoff block if one exists (identified by
// markers) rather than duplicating or clobbering the rest of the file.
// Returns the path written.
func WriteToTarget(to sources.Tool, dir, content string) (string, error) {
	filename, _ := TargetFile(to)
	path := filepath.Join(dir, filename)

	block := startMarker + "\n" + strings.TrimRight(content, "\n") + "\n" + endMarker

	existing, err := os.ReadFile(path)
	var next string
	switch {
	case os.IsNotExist(err):
		next = block + "\n"
	case err != nil:
		return "", fmt.Errorf("reading %s: %w", path, err)
	default:
		s := string(existing)
		if i := strings.Index(s, startMarker); i >= 0 {
			j := strings.Index(s, endMarker)
			if j < 0 || j < i {
				return "", fmt.Errorf("%s has a start marker but no matching end marker — refusing to overwrite; edit it by hand", path)
			}
			next = s[:i] + block + s[j+len(endMarker):]
		} else {
			sep := "\n"
			if strings.HasSuffix(s, "\n") || s == "" {
				sep = ""
			}
			next = s + sep + "\n" + block + "\n"
		}
	}

	if err := os.WriteFile(path, []byte(next), 0644); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	return path, nil
}
