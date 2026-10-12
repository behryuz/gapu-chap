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
			returning id, created_at`

	err := r.pool.QueryRow(ctx, query, u.FirstName, u.LastName, u.Username, u.PasswordHash).Scan(&u.ID, &u.CreatedAt)

	if err != nil {
		if hasPgCode(err, uniqueViolationCode) {
			return domain.ErrConflict
		}
		return fmt.Errorf("creating user: %w", err)
	}
	return nil
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	const query = `
		SELECT id, first_name, last_name, username, password_hash, created_at
		FROM users
		WHERE lower(username) = lower($1)`

	var u domain.User
	err := r.pool.QueryRow(ctx, query, username).Scan(&u.ID, &u.FirstName, &u.LastName, &u.Username, &u.PasswordHash, &u.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("select user by username: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	const query = `
		SELECT id, first_name, last_name, username, password_hash, created_at
		FROM users
		WHERE id = $1`

	var u domain.User
	err := r.pool.QueryRow(ctx, query, id).Scan(&u.ID, &u.FirstName, &u.LastName, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("select user by id: %w", err)
	}
	return &u, nil
}
