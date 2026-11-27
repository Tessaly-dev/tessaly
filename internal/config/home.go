// SPDX-License-Identifier: Apache-2.0

// Package config finds and prepares the folders where Tessaly keeps its data.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// HomeEnv names the environment variable that moves the data folder.
const HomeEnv = "TESSALY_HOME"

// Home returns the Tessaly data folder: $TESSALY_HOME if set, else ~/.tessaly.
// It does not create the folder. See EnsureHome.
func Home() (string, error) {
	if dir := os.Getenv(HomeEnv); dir != "" {
		if !filepath.IsAbs(dir) {
			return "", fmt.Errorf("%s must be an absolute path, got %q", HomeEnv, dir)
		}
		return filepath.Clean(dir), nil
	}
	user, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home folder: %w", err)
	}
	return filepath.Join(user, ".tessaly"), nil
}

// EnsureHome creates dir with mode 0700 if it does not exist. It refuses a
// folder that other users can read or write, because it holds the login token.
func EnsureHome(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	return CheckPrivate(dir)
}

// CheckPrivate returns an error unless path exists and only its owner can
// read, write or enter it.
func CheckPrivate(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf("%s has mode %04o, other users can reach it. Run: chmod go-rwx %s", path, perm, path)
	}
	return nil
}
