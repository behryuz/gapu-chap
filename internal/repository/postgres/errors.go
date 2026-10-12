package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	uniqueViolationCode     = "23505"
	foreignKeyViolationCode = "23503"
)

func hasPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
