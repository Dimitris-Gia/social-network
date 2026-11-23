package sqlite

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	sqlite3migrate "github.com/golang-migrate/migrate/v4/database/sqlite3"
)

// The migrations path is relative to backend/, so run the tests from there.
func init() {
	if err := os.Chdir("../../.."); err != nil {
		panic(err)
	}
}

func TestMigrationsUpConstraintsDown(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var version, dirty int
	if err := db.QueryRow(`SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty); err != nil {
		t.Fatal(err)
	}
	if version != 13 || dirty != 0 {
		t.Fatalf("schema_migrations = (%d, %d), want (13, 0)", version, dirty)
	}

	var fk int
	db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk)
	if fk != 1 {
		t.Fatal("foreign keys are off")
	}

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustFail := func(why, q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err == nil {
			t.Errorf("expected failure (%s): %s", why, q)
		}
	}

	user := `INSERT INTO users (email, password_hash, first_name, last_name, date_of_birth) VALUES (?, 'x', 'F', 'L', '2000-01-01')`
	mustExec(user, "a@x.com")
	mustExec(user, "b@x.com")
	mustFail("duplicate email", user, "a@x.com")
	mustFail("is_private not 0/1", `UPDATE users SET is_private = 2 WHERE id = 1`)

	mustExec(`INSERT INTO follows (follower_id, following_id, status) VALUES (1, 2, 'pending')`)
	mustFail("duplicate follow", `INSERT INTO follows (follower_id, following_id, status) VALUES (1, 2, 'accepted')`)
	mustFail("self follow", `INSERT INTO follows (follower_id, following_id, status) VALUES (1, 1, 'accepted')`)
	mustFail("bad follow status", `INSERT INTO follows (follower_id, following_id, status) VALUES (2, 1, 'maybe')`)
	mustFail("unknown user", `INSERT INTO follows (follower_id, following_id, status) VALUES (1, 99, 'pending')`)

	mustExec(`INSERT INTO groups (creator_id, title, description) VALUES (1, 'g', 'd')`)
	mustExec(`INSERT INTO posts (user_id, group_id, content, privacy) VALUES (1, 1, 'hi', 'public')`)
	mustFail("bad privacy", `INSERT INTO posts (user_id, content, privacy) VALUES (1, 'hi', 'secret')`)
	mustExec(`INSERT INTO comments (post_id, user_id, content) VALUES (1, 2, 'c')`)

	mustExec(`INSERT INTO messages (sender_id, receiver_id, content) VALUES (1, 2, 'pm')`)
	mustExec(`INSERT INTO messages (sender_id, group_id, content) VALUES (1, 1, 'gm')`)
	mustFail("message with both targets", `INSERT INTO messages (sender_id, receiver_id, group_id, content) VALUES (1, 2, 1, 'x')`)
	mustFail("message with no target", `INSERT INTO messages (sender_id, content) VALUES (1, 'x')`)

	mustExec(`INSERT INTO group_members (group_id, user_id, status, invited_by) VALUES (1, 2, 'invited', 1)`)
	mustExec(`INSERT INTO events (group_id, creator_id, title, description, event_time) VALUES (1, 1, 'e', 'd', '2026-12-01 18:00')`)
	rsvp := `INSERT INTO event_responses (event_id, user_id, response) VALUES (1, 2, ?)
	         ON CONFLICT(event_id, user_id) DO UPDATE SET response = excluded.response, updated_at = CURRENT_TIMESTAMP`
	mustExec(rsvp, "going")
	mustExec(rsvp, "not_going")
	mustFail("bad rsvp", `INSERT INTO event_responses (event_id, user_id, response) VALUES (1, 1, 'maybe')`)
	mustExec(`INSERT INTO notifications (user_id, actor_id, type, group_id, event_id) VALUES (2, 1, 'group_event', 1, 1)`)
	mustFail("bad notification type", `INSERT INTO notifications (user_id, type) VALUES (2, 'spam')`)

	// Deleting the group cascades to its posts (and their comments), events,
	// responses, members, group messages and notifications.
	mustExec(`DELETE FROM groups WHERE id = 1`)
	for _, table := range []string{"posts", "comments", "events", "event_responses", "group_members", "notifications"} {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n)
		if n != 0 {
			t.Errorf("%s: %d rows left after deleting the group", table, n)
		}
	}
	var msgs int
	db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&msgs)
	if msgs != 1 {
		t.Errorf("messages = %d, want 1 (only the private one)", msgs)
	}

	// Every down migration must reverse cleanly.
	driver, err := sqlite3migrate.WithInstance(db, &sqlite3migrate.Config{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.NewWithDatabaseInstance(MigrationsPath, "sqlite3", driver)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Down(); err != nil {
		t.Fatalf("down: %v", err)
	}
	// schema_migrations and its version_unique index belong to golang-migrate.
	var left string
	db.QueryRow(`SELECT COALESCE(group_concat(name), '') FROM sqlite_master
	              WHERE type IN ('table', 'index') AND name NOT LIKE 'sqlite_%'
	                AND name NOT IN ('schema_migrations', 'version_unique')`).Scan(&left)
	if left != "" {
		t.Errorf("left after down: %s", left)
	}
}
