// Package testdb даёт интеграционным тестам настоящий PostgreSQL.
//
// Каждый вызов New создаёт отдельную пустую базу, применяет к ней все миграции
// и удаляет её после теста. Тесты не мешают друг другу и могут идти параллельно.
//
// Нужна переменная TEST_DATABASE_URL — подключение пользователя с правом CREATE DATABASE
// (например, к служебной базе postgres). Без неё интеграционные тесты пропускаются,
// поэтому обычный `make test` остаётся быстрым и не требует базы.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gapu-chap/internal/repository/postgres"
)

const envVar = "TEST_DATABASE_URL"

func New(t *testing.T) *pgxpool.Pool {
	t.Helper()

	adminURL := os.Getenv(envVar)
	if adminURL == "" {
		t.Skipf("%s is not set: skipping integration test", envVar)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to %s: %v", envVar, err)
	}

	name := "test_" + randomSuffix(t)
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatalf("create test database: %v", err)
	}

	// Cleanup выполняются в обратном порядке: сначала закроется пул (ниже), потом удалим базу.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test database: %v", err)
		}
		_ = admin.Close(ctx)
	})

	dsn := withDatabase(t, adminURL, name)
	if err := postgres.Migrate(ctx, dsn); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	pool, err := postgres.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

func withDatabase(t *testing.T, rawURL, database string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %s: %v", envVar, err)
	}
	u.Path = "/" + database
	return u.String()
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("random: %v", err)
	}
	return hex.EncodeToString(b)
}
