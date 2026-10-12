-- +goose Up
CREATE TABLE rooms
(
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT        NOT NULL,
    owner_id   BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT rooms_name_length CHECK (char_length(name) BETWEEN 1 AND 64)
);

CREATE UNIQUE INDEX rooms_name_lower_idx ON rooms (lower(name));
CREATE INDEX rooms_owner_id_idx ON rooms (owner_id);

CREATE TABLE room_members
(
    room_id   BIGINT      NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    user_id   BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (room_id, user_id)
);

CREATE INDEX room_members_user_id_idx ON room_members (user_id);

-- +goose Down
DROP TABLE room_members;
DROP TABLE rooms;
