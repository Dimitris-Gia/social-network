-- Owner: A
CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    email         TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,
    first_name    TEXT    NOT NULL,
    last_name     TEXT    NOT NULL,
    date_of_birth TEXT    NOT NULL,              -- YYYY-MM-DD
    avatar_path   TEXT,
    nickname      TEXT,
    about_me      TEXT,
    is_private    INTEGER NOT NULL DEFAULT 0 CHECK (is_private IN (0, 1)),
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
