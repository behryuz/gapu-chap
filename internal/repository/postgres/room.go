package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gapu-chap/internal/domain"
)

type RoomRepository struct {
	pool *pgxpool.Pool
}

func NewRoomRepository(pool *pgxpool.Pool) *RoomRepository {
	return &RoomRepository{pool: pool}
}

const roomColumns = `id, name, owner_id, created_at`

func scanRoom(row pgx.Row) (domain.Room, error) {
	var room domain.Room
	err := row.Scan(&room.ID, &room.Name, &room.OwnerID, &room.CreatedAt)
	return room, err
}

func (r *RoomRepository) Create(ctx context.Context, room *domain.Room) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const insertRoom = `
		INSERT INTO rooms (name, owner_id)
		VALUES ($1, $2)
		RETURNING id, created_at`

	err = tx.QueryRow(ctx, insertRoom, room.Name, room.OwnerID).Scan(&room.ID, &room.CreatedAt)
	if err != nil {
		if hasPgCode(err, uniqueViolationCode) {
			return domain.ErrConflict
		}
		return fmt.Errorf("insert room: %w", err)
	}

	const insertOwner = `INSERT INTO room_members (room_id, user_id) VALUES ($1, $2)`

	if _, err = tx.Exec(ctx, insertOwner, room.ID, room.OwnerID); err != nil {
		return fmt.Errorf("insert owner as member: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (r *RoomRepository) GetByID(ctx context.Context, id int64) (*domain.Room, error) {
	const query = `SELECT ` + roomColumns + ` FROM rooms WHERE id = $1`

	room, err := scanRoom(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get room: %w", err)
	}
	return &room, nil
}

func (r *RoomRepository) List(ctx context.Context) ([]domain.Room, error) {
	const query = `SELECT ` + roomColumns + ` FROM rooms ORDER BY created_at DESC LIMIT 100`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list rooms: %w", err)
	}

	rooms, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Room, error) {
		return scanRoom(row)
	})

	if err != nil {
		return nil, fmt.Errorf("scan rooms: %w", err)
	}
	return rooms, nil
}

func (r *RoomRepository) Delete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM rooms WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete room: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *RoomRepository) AddMember(ctx context.Context, roomID, userID int64) error {
	const query = `
		INSERT INTO room_members (room_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT (room_id, user_id) DO NOTHING`

	if _, err := r.pool.Exec(ctx, query, roomID, userID); err != nil {
		if hasPgCode(err, foreignKeyViolationCode) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("insert room member: %w", err)
	}
	return nil
}

func (r *RoomRepository) RemoveMember(ctx context.Context, roomID, userID int64) error {
	const query = `DELETE FROM room_members WHERE room_id = $1 AND user_id = $2`

	if _, err := r.pool.Exec(ctx, query, roomID, userID); err != nil {
		return fmt.Errorf("delete room member: %w", err)
	}
	return nil
}

func (r *RoomRepository) ListMembers(ctx context.Context, roomID int64) ([]domain.Member, error) {
	const query = `
		SELECT u.id, u.username, u.first_name, u.last_name, m.joined_at, m.last_read_message_id
		FROM room_members m 
		JOIN users u ON u.id = m.user_id
		WHERE m.room_id = $1
		ORDER BY m.joined_at`

	rows, err := r.pool.Query(ctx, query, roomID)
	if err != nil {
		return nil, fmt.Errorf("list room members: %w", err)
	}

	members, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Member, error) {
		var m domain.Member
		err := row.Scan(&m.UserID, &m.Username, &m.FirstName, &m.LastName, &m.JoinedAt, &m.LastReadMessageID)
		return m, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan room members: %w", err)
	}
	return members, nil
}

func (r *RoomRepository) IsMember(ctx context.Context, roomID, userID int64) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM room_members WHERE room_id = $1 AND user_id = $2
		)`

	var isMember bool
	if err := r.pool.QueryRow(ctx, query, roomID, userID).Scan(&isMember); err != nil {
		return false, fmt.Errorf("check room membership: %w", err)
	}
	return isMember, nil
}

func (r *RoomRepository) MarkRead(ctx context.Context, roomID, userID, messageID int64) (bool, error) {
	const query = `
		UPDATE room_members
		SET last_read_message_id = $3
		WHERE room_id = $1 AND user_id = $2 AND last_read_message_id < $3`

	tag, err := r.pool.Exec(ctx, query, roomID, userID, messageID)
	if err != nil {
		return false, fmt.Errorf("update last read message: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *RoomRepository) ListForUser(ctx context.Context, userID int64) ([]domain.UserRoom, error) {
	const query = `
                SELECT r.id, r.name, r.owner_id, r.created_at, rm.last_read_message_id,
                       (SELECT count(*)
                        FROM messages m
                        WHERE m.room_id = r.id
                          AND m.id > rm.last_read_message_id
                          AND m.author_id <> rm.user_id) AS unread_count
                FROM room_members rm
                JOIN rooms r ON r.id = rm.room_id
                WHERE rm.user_id = $1
                ORDER BY r.id DESC`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list user rooms: %w", err)
	}

	rooms, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.UserRoom, error) {
		var ur domain.UserRoom
		err := row.Scan(&ur.ID, &ur.Name, &ur.OwnerID, &ur.CreatedAt, &ur.LastReadMessageID, &ur.UnreadCount)
		return ur, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan user rooms: %w", err)
	}
	return rooms, nil
}
