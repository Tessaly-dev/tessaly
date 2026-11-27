// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestGenericCommand(t *testing.T) {
	g := Generic{Argv: []string{"aider", "--message", "{prompt}", "--note={prompt}"}}
	prompt := `fix it; rm -rf ~ && echo "$HOME" --yes`

	got, err := g.Command(prompt)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"aider", "--message", prompt, "--note={prompt}"}
	if !slices.Equal(got, want) {
		t.Errorf("Command() = %q, want %q", got, want)
	}
	if g.Argv[2] != "{prompt}" {
		t.Error("Command changed the adapter's own argv")
	}
}

func TestGenericCommandWithoutPlaceholder(t *testing.T) {
	got, err := Generic{Argv: []string{"bash"}}.Command("ignored")
	if err != nil || !slices.Equal(got, []string{"bash"}) {
		t.Errorf("Command() = %q, %v", got, err)
	}
}

func TestGenericCommandRejectsBadPrompts(t *testing.T) {
	g := Generic{Argv: []string{"x", "{prompt}"}}
	for _, p := range []string{"a\x00b", strings.Repeat("a", MaxPromptBytes+1)} {
		if _, err := g.Command(p); !errors.Is(err, ErrBadPrompt) {
			t.Errorf("Command(%d bytes) err = %v, want ErrBadPrompt", len(p), err)
		}
	}
}
