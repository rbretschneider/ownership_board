-- Set when a provider stops listing a repo; cleared if it comes back. The repo
-- is kept because tickets may still point at it.
ALTER TABLE repos ADD COLUMN missing_since TEXT;
