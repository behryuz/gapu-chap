// Package migrations встраивает SQL-миграции в бинарник, чтобы их могли применить
// и сервер при старте (MIGRATE_ON_START=true, удобно в Docker), и интеграционные тесты.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
