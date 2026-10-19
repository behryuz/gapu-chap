-- +goose Up
-- Личный диалог (DM) — это обычная комната с kind = 'direct' и ровно двумя участниками.
-- dm_key = 'dm:<меньший id>:<больший id>' — уникальный ключ пары, поэтому второй диалог
-- между теми же людьми создать невозможно даже при одновременных запросах.
ALTER TABLE rooms
    ADD COLUMN kind   TEXT NOT NULL DEFAULT 'public',
    ADD COLUMN dm_key TEXT UNIQUE,
    ADD CONSTRAINT rooms_kind_check CHECK (kind IN ('public', 'direct')),
    ADD CONSTRAINT rooms_dm_key_check CHECK ((kind = 'direct') = (dm_key IS NOT NULL));

-- Уникальность названия нужна только публичным комнатам: у диалогов имя служебное.
DROP INDEX rooms_name_lower_idx;
CREATE UNIQUE INDEX rooms_name_lower_idx ON rooms (lower(name)) WHERE kind = 'public';

-- +goose Down
DELETE FROM rooms WHERE kind = 'direct';

DROP INDEX rooms_name_lower_idx;
CREATE UNIQUE INDEX rooms_name_lower_idx ON rooms (lower(name));

ALTER TABLE rooms
    DROP CONSTRAINT rooms_dm_key_check,
    DROP CONSTRAINT rooms_kind_check,
    DROP COLUMN dm_key,
    DROP COLUMN kind;
