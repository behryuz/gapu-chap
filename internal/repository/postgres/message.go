package postgres

import (
	"context"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gapu-chap/internal/domain"
)

type MessageRepository struct {
	pool *pgxpool.Pool
}

func NewMessageRepository(pool *pgxpool.Pool) *MessageRepository {
	return &MessageRepository{pool: pool}
}

func scanMessage(row pgx.Row) (domain.Message, error) {
	var m domain.Message
	err := row.Scan(&m.ID, &m.RoomID, &m.AuthorID, &m.Body, &m.CreatedAt)
	return m, err
}

func (r *MessageRepository) Create(ctx context.Context, msg *domain.Message) error {
	const query = `
		WITH inserted AS (
			INSERT INTO messages (room_id, author_id, body)
			VALUES ($1, $2, $3)
			RETURNING id, author_id, created_at
		)
		SELECT i.id, u.username, i.created_at
		FROM inserted i
		join users u on u.id = i.author_id`

	err := r.pool.QueryRow(ctx, query, msg.RoomID, msg.AuthorID, msg.Body).Scan(&msg.ID, &msg.AuthorUsername, &msg.CreatedAt)
	if err != nil {
		if hasPgCode(err, foreignKeyViolationCode) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}

func (r *MessageRepository) ListByRoom(ctx context.Context, roomID, beforeID int64, limit int) ([]domain.Message, error) {
	if beforeID <= 0 {
		beforeID = math.MaxInt64
	}

	const query = `
		SELECT m.id, m.room_id, m.author_id, u.username, m.body, m.created_at
		FROM messages m 
		JOIN users u ON m.author_id = u.id
		WHERE m.room_id = $1 AND m.id < $2
		ORDER BY m.id DESC
		LIMIT $3`

	rows, err := r.pool.Query(ctx, query, roomID, beforeID, limit)
	if err != nil {
		return nil, fmt.Errorf("select messages: %w", err)
	}

	messages, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Message, error) {
		return scanMessage(row)
	})
	if err != nil {
		return nil, fmt.Errorf("scan messages: %w", err)
	}
	return messages, nil
}
