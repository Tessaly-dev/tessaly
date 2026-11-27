// SPDX-License-Identifier: Apache-2.0

// Package agent turns a task prompt into the argv of an agent CLI.
// Adapters only build commands. They never run anything themselves.
package agent

import (
	"errors"
	"strings"
)

// PromptPlaceholder is the argv element that the prompt replaces.
const PromptPlaceholder = "{prompt}"

// MaxPromptBytes caps the prompt size.
const MaxPromptBytes = 32 << 10

// ErrBadPrompt means the prompt is too long or holds a NUL byte.
var ErrBadPrompt = errors.New("the prompt must be under 32 KiB and must not hold NUL bytes")

// Adapter builds the command that starts an agent on a task.
type Adapter interface {
	// Command returns the argv to run for prompt.
	Command(prompt string) ([]string, error)
}

// Generic runs any CLI from a fixed argv. It does no output parsing.
type Generic struct {
	Argv []string
}

// Command returns a copy of the argv with every element that is exactly
// "{prompt}" replaced by prompt. The prompt always stays one argument, so it
// cannot add flags or shell syntax. Placeholders inside a longer element are
// left as they are.
func (g Generic) Command(prompt string) ([]string, error) {
	if len(prompt) > MaxPromptBytes || strings.ContainsRune(prompt, 0) {
		return nil, ErrBadPrompt
	}
	argv := make([]string, len(g.Argv))
	for i, arg := range g.Argv {
		if arg == PromptPlaceholder {
			arg = prompt
		}
		argv[i] = arg
	}
	return argv, nil
}
