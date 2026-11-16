-- Friendly names for the board's runners (workers sharing an agent token as
-- "@worker/<token id>/<worker id>"). Each agent family has an ordered pool;
-- worker "<family>_<n>" takes the name at position n, so it keeps the same
-- name across restarts and re-registration. worker is NULL while a name is
-- free. The owner may rename an assigned runner; the row keeps its position.
CREATE TABLE runner_names (
 family TEXT NOT NULL CHECK(length(family) BETWEEN 1 AND 48),
 position INTEGER NOT NULL CHECK(position >= 1),
 name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 32),
 worker TEXT CHECK(worker IS NULL OR length(worker) BETWEEN 1 AND 48),
 assigned_at TEXT,
 PRIMARY KEY (family, position),
 UNIQUE (family, name),
 CHECK((worker IS NULL) = (assigned_at IS NULL))
);
CREATE UNIQUE INDEX runner_names_worker ON runner_names(worker) WHERE worker IS NOT NULL;

-- The claude family: The Office. "Ryan" is left out on purpose: it would read
-- as the board owner in reports.
INSERT INTO runner_names (family, position, name) VALUES
 ('claude', 1, 'Michael'), ('claude', 2, 'Dwight'), ('claude', 3, 'Jim'),
 ('claude', 4, 'Pam'), ('claude', 5, 'Angela'), ('claude', 6, 'Andy'),
 ('claude', 7, 'Kevin'), ('claude', 8, 'Oscar'), ('claude', 9, 'Stanley'),
 ('claude', 10, 'Phyllis'), ('claude', 11, 'Creed'), ('claude', 12, 'Kelly'),
 ('claude', 13, 'Toby'), ('claude', 14, 'Meredith'), ('claude', 15, 'Darryl'),
 ('claude', 16, 'Erin'), ('claude', 17, 'Jan'), ('claude', 18, 'Holly');

-- The codex family: Everybody Loves Raymond.
INSERT INTO runner_names (family, position, name) VALUES
 ('codex', 1, 'Ray'), ('codex', 2, 'Debra'), ('codex', 3, 'Robert'),
 ('codex', 4, 'Marie'), ('codex', 5, 'Frank'), ('codex', 6, 'Amy'),
 ('codex', 7, 'Ally'), ('codex', 8, 'Geoffrey'), ('codex', 9, 'Michael'),
 ('codex', 10, 'Peter'), ('codex', 11, 'Pat'), ('codex', 12, 'Gianni'),
 ('codex', 13, 'Nemo'), ('codex', 14, 'Andy');
