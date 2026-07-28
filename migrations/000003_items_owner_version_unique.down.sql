ALTER TABLE items DROP CONSTRAINT IF EXISTS items_owner_version_unique;
CREATE INDEX IF NOT EXISTS idx_items_owner_version ON items (owner_id, version);
