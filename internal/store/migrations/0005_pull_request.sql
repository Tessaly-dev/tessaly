-- SPDX-License-Identifier: Apache-2.0
-- The pull request opened from a workspace's branch, if any.
ALTER TABLE workspaces ADD COLUMN pr_url TEXT NOT NULL DEFAULT '';
