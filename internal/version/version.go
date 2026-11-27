// SPDX-License-Identifier: Apache-2.0

// Package version holds the build version of Tessaly.
package version

// Version is set at build time with
// -ldflags "-X github.com/tessaly-dev/tessaly/internal/version.Version=v0.1.0".
var Version = "dev"

// String returns the version text printed by "tess version".
func String() string {
	return "tess " + Version
}
