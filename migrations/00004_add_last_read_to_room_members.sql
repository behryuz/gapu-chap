-- +goose Up
ALTER TABLE room_members
    ADD COLUMN last_read_message_id BIGINT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE room_members
    DROP COLUMN last_read_message_id;
