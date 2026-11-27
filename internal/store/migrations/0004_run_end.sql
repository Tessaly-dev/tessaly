-- SPDX-License-Identifier: Apache-2.0
-- How the last agent run ended: 'exited' on its own, 'stopped' by the user,
-- or 'interrupted' because tess stopped. Empty while one runs or before the
-- first run.
ALTER TABLE workspaces ADD COLUMN ended TEXT NOT NULL DEFAULT '';
