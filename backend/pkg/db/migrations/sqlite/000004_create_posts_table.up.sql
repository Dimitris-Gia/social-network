-- Owner: B
-- groups is created in 000007. SQLite only checks the foreign key when a row
-- is inserted, by which time every migration has run.
CREATE TABLE posts (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id   INTEGER REFERENCES groups(id) ON DELETE CASCADE,  -- NULL = not a group post
    content    TEXT    NOT NULL,
    image_path TEXT,
    privacy    TEXT    NOT NULL DEFAULT 'public'
               CHECK (privacy IN ('public', 'almost_private', 'private')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_posts_user_id ON posts(user_id);
CREATE INDEX idx_posts_group_id ON posts(group_id);
