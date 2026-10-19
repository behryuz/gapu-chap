-- +goose Up
-- Поисковый вектор считает сама база при каждой вставке (GENERATED ... STORED),
-- поэтому код приложения не может «забыть» его обновить.
-- Словарь 'simple' не делает стемминг, зато одинаково работает для русского и английского;
-- поиск по началу слова (prefix) добавляем в запросе: «прив» найдёт «привет».
-- Внимание: добавление STORED-колонки переписывает таблицу — на большой базе это долго.
ALTER TABLE messages
    ADD COLUMN search_vector tsvector
        GENERATED ALWAYS AS (to_tsvector('simple', body)) STORED;

CREATE INDEX messages_search_vector_idx ON messages USING GIN (search_vector);

-- +goose Down
DROP INDEX messages_search_vector_idx;
ALTER TABLE messages DROP COLUMN search_vector;
