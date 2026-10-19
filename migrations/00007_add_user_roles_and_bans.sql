-- +goose Up
-- banned_at вместо булева флага: видно не только «заблокирован ли», но и когда.
-- Модератора назначают вручную (make promote username=...), через API роль не меняется.
ALTER TABLE users
    ADD COLUMN role      TEXT NOT NULL DEFAULT 'user',
    ADD COLUMN banned_at TIMESTAMPTZ,
    ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'moderator'));

-- +goose Down
ALTER TABLE users
    DROP CONSTRAINT users_role_check,
    DROP COLUMN banned_at,
    DROP COLUMN role;
