-- The owner's opt-in, per connection, to let board agents (workers and the
-- merge sweep) borrow the connection's token to clone, push and open pull
-- requests. Only Forgejo connections can turn it on; off by default.
ALTER TABLE provider_connections ADD COLUMN allow_agent_access INTEGER NOT NULL DEFAULT 0 CHECK (allow_agent_access IN (0, 1));
