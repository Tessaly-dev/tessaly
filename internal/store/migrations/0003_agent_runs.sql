-- SPDX-License-Identifier: Apache-2.0
-- The agent that last ran in a workspace, and how it ended.
ALTER TABLE workspaces ADD COLUMN agent TEXT NOT NULL DEFAULT '';
ALTER TABLE workspaces ADD COLUMN exit_code INTEGER;
