-- SPDX-License-Identifier: Apache-2.0
-- An audit log of sign-in resets made from the host.
CREATE TABLE auth_events (
    id       INTEGER PRIMARY KEY,
    at       TEXT NOT NULL,
    kind     TEXT NOT NULL,
    method   TEXT NOT NULL,
    passkeys INTEGER NOT NULL,
    sessions INTEGER NOT NULL
);
