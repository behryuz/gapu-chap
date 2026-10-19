package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gapu-chap/internal/domain"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Create(ctx context.Context, u *domain.User) error {
	const query = `
			INSERT INTO users (first_name, last_name, username, password_hash)
			VALUES ($1, $2, $3, $4)
			returning id, role, created_at`

	err := r.pool.QueryRow(ctx, query, u.FirstName, u.LastName, u.Username, u.PasswordHash).Scan(&u.ID, &u.Role, &u.CreatedAt)

	if err != nil {
		if hasPgCode(err, uniqueViolationCode) {
			return domain.ErrConflict
		}
		return fmt.Errorf("creating user: %w", err)
	}
	return nil
}

const userColumns = `id, first_name, last_name, username, password_hash, role, banned_at, created_at`

func scanUser(row pgx.Row) (domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.FirstName, &u.LastName, &u.Username, &u.PasswordHash,
		&u.Role, &u.BannedAt, &u.CreatedAt)
	return u, err
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	const query = `SELECT ` + userColumns + ` FROM users WHERE lower(username) = lower($1)`

	u, err := scanUser(r.pool.QueryRow(ctx, query, username))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("select user by username: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	const query = `SELECT ` + userColumns + ` FROM users WHERE id = $1`

	u, err := scanUser(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("select user by id: %w", err)
	}
	return &u, nil
}

// Search ищет по началу username, имени или фамилии без учёта регистра.
// starts_with вместо LIKE: в запросе пользователя могут быть «%» и «_»,
// которые для LIKE — спецсимволы и требуют экранирования.
func (r *UserRepository) Search(ctx context.Context, query string, limit int) ([]domain.User, error) {
	const sql = `
		SELECT ` + userColumns + `
		FROM users
		WHERE starts_with(lower(username), lower($1))
		   OR starts_with(lower(first_name), lower($1))
		   OR starts_with(lower(last_name), lower($1))
		ORDER BY username
		LIMIT $2`

	rows, err := r.pool.Query(ctx, sql, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	users, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.User, error) {
		return scanUser(row)
	})
	if err != nil {
		return nil, fmt.Errorf("scan users: %w", err)
	}
	return users, nil
}

// SetBanned блокирует (banned = true) или разблокирует пользователя.
// Повторная блокировка не сдвигает дату: COALESCE оставляет исходный banned_at.
func (r *UserRepository) SetBanned(ctx context.Context, id int64, banned bool) error {
	const query = `
		UPDATE users
		SET banned_at = CASE WHEN $2 THEN COALESCE(banned_at, now()) END
		WHERE id = $1`

	tag, err := r.pool.Exec(ctx, query, id, banned)
	if err != nil {
		return fmt.Errorf("update user ban: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
