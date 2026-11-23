-- Owner: B
-- Only used when posts.privacy = 'private'.
CREATE TABLE post_allowed_viewers (
    post_id INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (post_id, user_id)
);
