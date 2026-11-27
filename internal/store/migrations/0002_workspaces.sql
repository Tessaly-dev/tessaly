-- SPDX-License-Identifier: Apache-2.0
-- A workspace is one task: one branch and one worktree in one repo.
CREATE TABLE workspaces (
	id         INTEGER PRIMARY KEY,
	repo_id    INTEGER NOT NULL REFERENCES repos (id) ON DELETE RESTRICT,
	slug       TEXT NOT NULL,
	branch     TEXT NOT NULL,
	base       TEXT NOT NULL,
	path       TEXT NOT NULL UNIQUE,
	state      TEXT NOT NULL,
	error      TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	UNIQUE (repo_id, slug)
) STRICT;
