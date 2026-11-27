// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the global config file, ~/.tessaly/config.toml.
type Config struct {
	// Agents maps an agent name to how to run it.
	Agents map[string]Agent `toml:"agents"`
}

// Agent is an agent CLI that the generic adapter runs.
type Agent struct {
	// Command is the argv to run. It never goes through a shell. An element
	// that is exactly "{prompt}" is replaced by the task prompt.
	Command []string `toml:"command"`
	// Resume is the argv that picks up the agent's last session in the
	// worktree, such as ["claude", "--continue"]. It is optional, and it
	// takes no placeholders, because Tessaly does not keep the prompt.
	Resume []string `toml:"resume"`
}

var agentNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// Load reads the config file at path. A missing file gives an empty config.
// Because the file says which programs Tessaly runs, Load refuses a file that
// other users can write.
func Load(path string) (Config, error) {
	var cfg Config
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if perm := info.Mode().Perm(); perm&0o022 != 0 {
		return cfg, fmt.Errorf("%s has mode %04o, other users can change it. Run: chmod go-w %s", path, perm, path)
	}

	md, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return cfg, fmt.Errorf("read %s: unknown setting %q", path, undecoded[0].String())
	}
	if err := cfg.validate(); err != nil {
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}
	return cfg, nil
}

func (c Config) validate() error {
	for name, a := range c.Agents {
		if !agentNameRE.MatchString(name) {
			return fmt.Errorf("agent name %q must be 1 to 32 lower-case letters, digits or dashes", name)
		}
		if len(a.Command) == 0 || a.Command[0] == "" {
			return fmt.Errorf("agent %q needs a command, such as command = [\"aider\"]", name)
		}
		for _, arg := range a.Command {
			if strings.ContainsRune(arg, 0) {
				return fmt.Errorf("agent %q: command holds a NUL byte", name)
			}
		}
		if len(a.Resume) > 0 && a.Resume[0] == "" {
			return fmt.Errorf("agent %q: resume must start with a program", name)
		}
		for _, arg := range a.Resume {
			if strings.ContainsRune(arg, 0) {
				return fmt.Errorf("agent %q: resume holds a NUL byte", name)
			}
			if arg == "{prompt}" || arg == "{prompt_file}" {
				return fmt.Errorf("agent %q: resume cannot use %s, because Tessaly does not keep the prompt", name, arg)
			}
		}
	}
	return nil
}
