-- +goose Up
CREATE TABLE users
(
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    first_name    TEXT        NOT NULL,
    last_name     TEXT        NOT NULL,
    username      TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT users_username_length CHECK (char_length(username) BETWEEN 3 AND 32)
);

CREATE UNIQUE INDEX users_username_lower_idx ON users (lower(username));

-- +goose Down
DROP TABLE users;