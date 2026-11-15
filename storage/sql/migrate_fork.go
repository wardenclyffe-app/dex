package sql

import (
	"database/sql"
	"fmt"
)

// The NetBird fork of Dex diverged from upstream at migration 27: it added
// auth_session.connector_data (27) and, on SQLite only, cast password groups
// to BLOB (28). Upstream's 27 adds client columns and its 28 rebuilds
// auth_session. Migrations are applied by number, so a database created by the
// fork already counts those numbers as done and would skip upstream's steps.
//
// forkDivergence is the zero-based index of upstream's migration 27; upstream
// only ever appends, so these two entries keep their positions.
const (
	forkDivergence      = 26
	forkDivergenceSteps = 2
)

// reconcileForkedSchema applies upstream migrations 27 and 28 once to a
// database created by the NetBird fork. It recognises such a database by its
// schema rather than its migration count: auth_session has the fork's
// connector_data column and client lacks upstream's backchannel_logout_uri.
func (c *conn) reconcileForkedSchema() error {
	if !c.hasColumn("auth_session", "connector_data") || c.hasColumn("client", "backchannel_logout_uri") {
		return nil
	}

	c.logger.Info("reconciling a database created by the NetBird fork of Dex; signed-in sessions are reset once")
	return c.ExecTx(func(tx *trans) error {
		for _, m := range migrations[forkDivergence : forkDivergence+forkDivergenceSteps] {
			for i, stmt := range m.stmts {
				if _, err := tx.Exec(stmt); err != nil {
					return fmt.Errorf("reconcile fork schema: migration %d statement %d: %v", forkDivergence+1, i+1, err)
				}
			}
		}

		// The migrations table counts only the steps for this flavor, so the
		// target is how many of upstream's first 28 apply here. A Postgres fork
		// database is one short (the fork's 28 was SQLite only); a SQLite one
		// already counts its own 28, which upstream runs later as a harmless
		// re-cast.
		target := c.flavorStepsThrough(forkDivergence + forkDivergenceSteps)
		var num sql.NullInt64
		if err := tx.QueryRow(`select max(num) from migrations;`).Scan(&num); err != nil {
			return fmt.Errorf("reconcile fork schema: select max migration: %v", err)
		}
		if !num.Valid || num.Int64 < int64(target) {
			if _, err := tx.Exec(`insert into migrations (num, at) values ($1, now());`, target); err != nil {
				return fmt.Errorf("reconcile fork schema: record migration: %v", err)
			}
		}
		return nil
	})
}

// flavorStepsThrough counts the migrations among the first n that apply to
// this connection's flavor.
func (c *conn) flavorStepsThrough(n int) int {
	steps := 0
	for _, m := range migrations[:n] {
		if m.flavor == nil || m.flavor == c.flavor {
			steps++
		}
	}
	return steps
}

// hasColumn reports whether table has column. A missing table or column is
// reported as false. It runs outside any transaction, because a failed
// statement aborts the surrounding transaction on Postgres.
func (c *conn) hasColumn(table, column string) bool {
	rows, err := c.Query(fmt.Sprintf("select %s from %s where 1 = 0;", column, table))
	if err != nil {
		return false
	}
	_ = rows.Close()
	return true
}
