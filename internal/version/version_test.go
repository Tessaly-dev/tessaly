// SPDX-License-Identifier: Apache-2.0

package version

import "testing"

func TestString(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })

	tests := []struct {
		version string
		want    string
	}{
		{"dev", "tess dev"},
		{"v0.1.0", "tess v0.1.0"},
	}
	for _, tt := range tests {
		Version = tt.version
		if got := String(); got != tt.want {
			t.Errorf("String() with Version=%q = %q, want %q", tt.version, got, tt.want)
		}
	}
}
