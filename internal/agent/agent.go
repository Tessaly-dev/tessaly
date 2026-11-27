// SPDX-License-Identifier: Apache-2.0

// Package agent turns a task prompt into the argv of an agent CLI.
// Adapters only build commands. They never run anything themselves.
package agent

import (
	"errors"
	"slices"
	"strings"
)

// PromptPlaceholder is the argv element that the prompt replaces.
const PromptPlaceholder = "{prompt}"

// PromptFilePlaceholder is the argv element that the path of a file holding
// the prompt replaces. Unlike an argument, a file does not show in ps.
const PromptFilePlaceholder = "{prompt_file}"

// MaxPromptBytes caps the prompt size.
const MaxPromptBytes = 32 << 10

// ErrBadPrompt means the prompt is too long or holds a NUL byte.
var ErrBadPrompt = errors.New("the prompt must be under 32 KiB and must not hold NUL bytes")

// Adapter builds the command that starts an agent on a task.
type Adapter interface {
	// Command returns the argv to run for prompt. promptFile is the path
	// where the caller writes the prompt if UsesPromptFile is true.
	Command(prompt, promptFile string) ([]string, error)
	// UsesPromptFile reports whether the command reads the prompt from a file.
	UsesPromptFile() bool
}

// Generic runs any CLI from a fixed argv. It does no output parsing.
type Generic struct {
	Argv []string
}

// Command returns a copy of the argv with every element that is exactly
// "{prompt}" replaced by prompt, and every element that is exactly
// "{prompt_file}" replaced by promptFile. The prompt always stays one
// argument, so it cannot add flags or shell syntax. Placeholders inside a
// longer element are left as they are.
func (g Generic) Command(prompt, promptFile string) ([]string, error) {
	if len(prompt) > MaxPromptBytes || strings.ContainsRune(prompt, 0) {
		return nil, ErrBadPrompt
	}
	argv := make([]string, len(g.Argv))
	for i, arg := range g.Argv {
		switch arg {
		case PromptPlaceholder:
			arg = prompt
		case PromptFilePlaceholder:
			arg = promptFile
		}
		argv[i] = arg
	}
	return argv, nil
}

// UsesPromptFile reports whether the argv holds "{prompt_file}".
func (g Generic) UsesPromptFile() bool { return slices.Contains(g.Argv, PromptFilePlaceholder) }
