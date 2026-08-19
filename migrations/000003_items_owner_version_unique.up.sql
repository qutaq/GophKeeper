-- Ensure (owner_id, version) uniqueness for DBs that applied an older 000001
-- with a non-unique index on these columns.
DROP INDEX IF EXISTS idx_items_owner_version;

ALTER TABLE items DROP CONSTRAINT IF EXISTS items_owner_version_unique;
ALTER TABLE items ADD CONSTRAINT items_owner_version_unique UNIQUE (owner_id, version);
