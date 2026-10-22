-- Confluence page history: who created / last updated the page and when
-- (from the Confluence API, not the local save time in created_at/updated_at).
ALTER TABLE pages ADD COLUMN confluence_created_by TEXT NOT NULL DEFAULT '';
ALTER TABLE pages ADD COLUMN confluence_created_at TEXT;
ALTER TABLE pages ADD COLUMN confluence_updated_by TEXT NOT NULL DEFAULT '';
ALTER TABLE pages ADD COLUMN confluence_updated_at TEXT;
