-- +goose Up
CREATE TABLE messages
(
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id    BIGINT      NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    author_id  BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    body       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT messages_body_length CHECK (char_length(body) BETWEEN 1 AND 4000)
);

CREATE INDEX messages_room_id_id_idx ON messages (room_id, id DESC);
CREATE INDEX messages_author_id_idx ON messages (author_id);

-- +goose Down
DROP TABLE messages
