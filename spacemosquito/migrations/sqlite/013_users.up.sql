-- Confluence user display names by account ID, for @-mentions (Cloud stores
-- only the account ID in page content).
CREATE TABLE users (
    account_id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
