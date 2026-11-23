-- Owner: C
-- Switch answer with:
--   INSERT ... ON CONFLICT(event_id, user_id) DO UPDATE SET response = excluded.response
CREATE TABLE event_responses (
    event_id   INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    response   TEXT    NOT NULL CHECK (response IN ('going', 'not_going')),
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (event_id, user_id)
);
