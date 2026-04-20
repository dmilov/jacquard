-- Jacquard SQLite schema (reference copy — auto-applied on startup via store.Migrate)

CREATE TABLE IF NOT EXISTS conversations (
    id         TEXT     NOT NULL PRIMARY KEY,
    node_id    TEXT     NOT NULL,
    command    TEXT     NOT NULL,
    started_at DATETIME NOT NULL,
    ended_at   DATETIME
);

CREATE INDEX IF NOT EXISTS idx_node_started
    ON conversations(node_id, started_at);

CREATE TABLE IF NOT EXISTS messages (
    id              INTEGER  PRIMARY KEY AUTOINCREMENT,
    conversation_id TEXT     NOT NULL,
    role            TEXT     NOT NULL CHECK(role IN ('user','assistant')),
    content         TEXT     NOT NULL,
    sequence        INTEGER  NOT NULL,
    created_at      DATETIME NOT NULL,
    FOREIGN KEY (conversation_id)
        REFERENCES conversations(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_conv_seq
    ON messages(conversation_id, sequence);
