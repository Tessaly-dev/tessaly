// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{"version", []string{"version"}, 0, "tess dev\n", ""},
		{"version flag", []string{"--version"}, 0, "tess dev\n", ""},
		{"help", []string{"help"}, 0, "Usage: tess", ""},
		{"no args", nil, 2, "", "Usage: tess"},
		{"unknown", []string{"nope"}, 2, "", `unknown command "nope"`},
		{"serve bad port", []string{"serve", "--port", "0"}, 2, "", "out of range"},
		{"serve high port", []string{"serve", "--port", "70000"}, 2, "", "out of range"},
		{"serve extra arg", []string{"serve", "0.0.0.0"}, 2, "", "unexpected argument"},
		{"serve unknown flag", []string{"serve", "--addr", "0.0.0.0:7777"}, 2, "", "flag provided but not defined"},
		{"serve help", []string{"serve", "-h"}, 0, "", "-port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(context.Background(), tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr: %s)", code, tt.wantCode, stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestServeStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Serve should start, see the cancelled context and shut down cleanly.

	var stdout, stderr bytes.Buffer
	// Port 0 is rejected, so pick a high port that is unlikely to be in use.
	code := run(ctx, []string{"serve", "--port", "47777"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "listening on http://127.0.0.1:47777") {
		t.Errorf("stdout = %q, want listen line", stdout.String())
	}
}
