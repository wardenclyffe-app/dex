//go:build cgo
// +build cgo

package sql

import (
	"database/sql"
	"log/slog"
	"testing"
)

func newSQLiteConn(t *testing.T) *conn {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1) // keep one in-memory database across statements
	t.Cleanup(func() { _ = db.Close() })
	return &conn{db, &flavorSQLite3, slog.New(slog.DiscardHandler), func(error) bool { return false }}
}

func TestMigrate_UpgradesNetBirdForkDatabase(t *testing.T) {
	c := newSQLiteConn(t)

	upstream := migrations
	migrations = netbirdForkMigrations()
	if _, err := c.migrate(); err != nil {
		migrations = upstream
		t.Fatalf("build fork-era database: %v", err)
	}
	migrations = upstream

	if !c.hasColumn("auth_session", "connector_data") || c.hasColumn("client", "backchannel_logout_uri") {
		t.Fatal("precondition: database should have the fork's schema")
	}

	if _, err := c.migrate(); err != nil {
		t.Fatalf("migrate fork-era database: %v", err)
	}

	for _, col := range []struct{ table, column string }{
		{"client", "backchannel_logout_uri"},
		{"client", "refresh_token_lifetime"},
		{"auth_session", "secret"},
		{"auth_session", "id"},
		{"auth_code", "session_id"},
	} {
		if !c.hasColumn(col.table, col.column) {
			t.Errorf("after upgrade, %s.%s is missing", col.table, col.column)
		}
	}
	if c.hasColumn("auth_session", "connector_data") {
		t.Error("after upgrade, auth_session should be upstream's rebuilt table")
	}

	var num int
	if err := c.QueryRow(`select max(num) from migrations;`).Scan(&num); err != nil {
		t.Fatal(err)
	}
	if want := flavorCount(migrations, &flavorSQLite3); num != want {
		t.Errorf("migration count = %d, want %d", num, want)
	}

	if n, err := c.migrate(); err != nil || n != 0 {
		t.Errorf("third migrate should be a no-op, ran %d, err %v", n, err)
	}
}

func TestMigrate_FreshDatabaseIsNotReconciled(t *testing.T) {
	c := newSQLiteConn(t)
	n, err := c.migrate()
	if err != nil {
		t.Fatal(err)
	}
	if want := flavorCount(migrations, &flavorSQLite3); n != want {
		t.Errorf("fresh database ran %d migrations, want %d", n, want)
	}
	if c.hasColumn("auth_session", "connector_data") {
		t.Error("fresh database must not have the fork's column")
	}
}
