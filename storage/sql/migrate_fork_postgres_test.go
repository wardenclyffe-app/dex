package sql

import (
	"log/slog"
	"os"
	"strconv"
	"testing"
)

// netbirdForkMigrations is the migration list the NetBird fork shipped: the
// first 26 upstream steps, then its own 27 and (SQLite only) 28.
func netbirdForkMigrations() []migration {
	fork := append([]migration{}, migrations[:forkDivergence]...)
	return append(fork,
		migration{stmts: []string{`alter table auth_session add column connector_data bytea;`}},
		migration{stmts: []string{`update password set groups = CAST(groups as BLOB);`}, flavor: &flavorSQLite3},
	)
}

func flavorCount(list []migration, f *flavor) int {
	n := 0
	for _, m := range list {
		if m.flavor == nil || m.flavor == f {
			n++
		}
	}
	return n
}

// The NetBird fork's SQLite-only 28th step leaves Postgres fork databases one
// migration short of SQLite ones, so the Postgres path needs its own proof.
func TestMigrate_UpgradesNetBirdForkDatabase_Postgres(t *testing.T) {
	host := os.Getenv(testPostgresEnv)
	if host == "" {
		t.Skipf("test environment variable %q not set, skipping", testPostgresEnv)
	}
	port := uint64(5432)
	if rawPort := os.Getenv("DEX_POSTGRES_PORT"); rawPort != "" {
		var err error
		if port, err = strconv.ParseUint(rawPort, 10, 32); err != nil {
			t.Fatalf("invalid postgres port %q: %s", rawPort, err)
		}
	}
	cfg := &Postgres{
		NetworkDB: NetworkDB{
			Database: getenv("DEX_POSTGRES_DATABASE", "postgres"),
			User:     getenv("DEX_POSTGRES_USER", "postgres"),
			Password: getenv("DEX_POSTGRES_PASSWORD", "postgres"),
			Host:     host,
			Port:     uint16(port),
		},
		SSL: SSL{Mode: pgSSLDisable},
	}
	logger := slog.New(slog.DiscardHandler)

	// Start from an empty schema with the fork's migration list.
	upstream := migrations
	migrations = []migration{}
	c, err := cfg.open(logger)
	migrations = upstream
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := c.Exec(`drop schema public cascade; create schema public;`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	_ = c.Close()

	migrations = netbirdForkMigrations()
	c, err = cfg.open(logger)
	migrations = upstream
	if err != nil {
		t.Fatalf("build fork-era database: %v", err)
	}
	var forkNum int
	if err := c.QueryRow(`select max(num) from migrations;`).Scan(&forkNum); err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
	if want := flavorCount(netbirdForkMigrations(), &flavorPostgres); forkNum != want {
		t.Fatalf("precondition: fork database recorded %d migrations, want %d", forkNum, want)
	}

	c, err = cfg.open(logger)
	if err != nil {
		t.Fatalf("migrate fork-era database: %v", err)
	}
	defer c.Close()

	for _, col := range []struct{ table, column string }{
		{"client", "backchannel_logout_uri"},
		{"client", "refresh_token_lifetime"},
		{"auth_session", "secret"},
		{"auth_code", "session_id"},
	} {
		if !c.hasColumn(col.table, col.column) {
			t.Errorf("after upgrade, %s.%s is missing", col.table, col.column)
		}
	}
	var num int
	if err := c.QueryRow(`select max(num) from migrations;`).Scan(&num); err != nil {
		t.Fatal(err)
	}
	if want := flavorCount(migrations, &flavorPostgres); num != want {
		t.Errorf("migration count = %d, want %d", num, want)
	}
}
