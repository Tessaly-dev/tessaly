// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "none.toml"))
	if err != nil || len(cfg.Agents) != 0 {
		t.Errorf("Load(missing) = %+v, %v; want empty config", cfg, err)
	}
}

func TestLoadAgents(t *testing.T) {
	path := writeConfig(t, `
[agents.aider]
command = ["aider", "--message", "{prompt}"]
resume = ["aider", "--restore-chat-history"]

[agents.shell-bot]
command = ["/usr/local/bin/bot"]
`, 0o644)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Agents["aider"].Command; len(got) != 3 || got[2] != "{prompt}" {
		t.Errorf("aider command = %q", got)
	}
	if got := cfg.Agents["aider"].Resume; len(got) != 2 || got[1] != "--restore-chat-history" {
		t.Errorf("aider resume = %q", got)
	}
	if bot, ok := cfg.Agents["shell-bot"]; !ok || bot.Resume != nil {
		t.Errorf("shell-bot = %+v, %v; want it with no resume", bot, ok)
	}
}

func TestLoadRejects(t *testing.T) {
	tests := []struct {
		name, body string
		mode       os.FileMode
		wantErr    string
	}{
		{"writable by others", "", 0o666, "other users can change it"},
		{"writable by group", "", 0o620, "other users can change it"},
		{"bad toml", "agents = [", 0o600, "read"},
		{"unknown key", "[agents.x]\ncommand = [\"x\"]\nshell = true\n", 0o600, "unknown setting"},
		{"bad agent name", "[agents.\"My Agent\"]\ncommand = [\"x\"]\n", 0o600, "agent name"},
		{"empty command", "[agents.x]\ncommand = []\n", 0o600, "needs a command"},
		{"blank program", "[agents.x]\ncommand = [\"\"]\n", 0o600, "needs a command"},
		{"string command", "[agents.x]\ncommand = \"rm -rf ~\"\n", 0o600, "read"},
		{"blank resume program", "[agents.x]\ncommand = [\"x\"]\nresume = [\"\"]\n", 0o600, "resume must start"},
		{"resume with prompt", "[agents.x]\ncommand = [\"x\"]\nresume = [\"x\", \"{prompt}\"]\n", 0o600, "does not keep the prompt"},
		{"resume with prompt file", "[agents.x]\ncommand = [\"x\"]\nresume = [\"x\", \"{prompt_file}\"]\n", 0o600, "does not keep the prompt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, tt.body, tt.mode))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestPublicURL(t *testing.T) {
	cfg, err := Load(writeConfig(t, `public_url = "https://Tess.Example.com/"`+"\n", 0o600))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Public(); got == nil || got.String() != "https://tess.example.com" {
		t.Errorf("Public() = %v, want https://tess.example.com", got)
	}
	if (Config{}).Public() != nil {
		t.Error("an empty public_url gives a URL")
	}
	for _, bad := range []string{
		"tess.example.com", "ftp://tess.example.com", "https://", "https://u:p@tess.example.com",
		"https://tess.example.com/tess", "https://tess.example.com/?a=1", "https://tess.example.com/#x",
	} {
		if _, err := ParsePublicURL(bad); err == nil {
			t.Errorf("ParsePublicURL(%q) passed, want an error", bad)
		}
		if _, err := Load(writeConfig(t, `public_url = "`+bad+`"`+"\n", 0o600)); err == nil {
			t.Errorf("Load with public_url %q passed, want an error", bad)
		}
	}
	if u, err := ParsePublicURL("http://tess.tailnet:7777"); err != nil || u.Host != "tess.tailnet:7777" {
		t.Errorf("ParsePublicURL with a port = %v, %v", u, err)
	}
}
