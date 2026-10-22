-- Confluence Cloud folders: containers in the page tree that are not pages
-- (no content). Pages inside a folder record it as their parent.
CREATE TABLE folders (
    space_id TEXT NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    confluence_id INTEGER NOT NULL,
    title TEXT NOT NULL,
    parent_confluence_id INTEGER,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (space_id, confluence_id)
);
