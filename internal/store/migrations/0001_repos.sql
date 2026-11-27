-- SPDX-License-Identifier: Apache-2.0
CREATE TABLE repos (
	id       INTEGER PRIMARY KEY,
	name     TEXT NOT NULL UNIQUE,
	path     TEXT NOT NULL UNIQUE,
	added_at TEXT NOT NULL
) STRICT;
