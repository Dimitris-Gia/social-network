-- Owner: A
CREATE TABLE notifications (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,  -- recipient
    actor_id   INTEGER REFERENCES users(id) ON DELETE CASCADE,           -- who caused it
    type       TEXT    NOT NULL CHECK (type IN (
                   'follow_request', 'group_invite', 'group_join_request', 'group_event',
                   'follow_accepted', 'new_follower')),
    group_id   INTEGER REFERENCES groups(id) ON DELETE CASCADE,
    event_id   INTEGER REFERENCES events(id) ON DELETE CASCADE,
    is_read    INTEGER NOT NULL DEFAULT 0 CHECK (is_read IN (0, 1)),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_notifications_user_read ON notifications(user_id, is_read);
