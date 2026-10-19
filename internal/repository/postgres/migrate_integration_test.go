package postgres_test

import (
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"gapu-chap/internal/testutil/testdb"
	"gapu-chap/migrations"
)

// Каждая миграция должна откатываться: Down до нуля и снова Up без ошибок.
// Иначе `make migrate-down` сломается ровно тогда, когда он понадобится.
func TestMigrations_DownAndUpAgain(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t) // база уже на последней версии

	db, err := sql.Open("pgx", pool.Config().ConnString())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	if _, err := provider.DownTo(t.Context(), 0); err != nil {
		t.Fatalf("down to 0: %v", err)
	}
	if _, err := provider.Up(t.Context()); err != nil {
		t.Fatalf("up again: %v", err)
	}
}
