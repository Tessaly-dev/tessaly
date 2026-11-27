// SPDX-License-Identifier: Apache-2.0
// Vite empties dist/ on build. Put back the placeholder that lets the Go
// embed directive compile before anyone has built the UI.
import { writeFileSync } from "node:fs";

writeFileSync(new URL("../dist/.gitkeep", import.meta.url), "");
