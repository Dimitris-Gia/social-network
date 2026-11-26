package models_test

import (
	"os"
	"path/filepath"
	"testing"

	"social-network/pkg/db/sqlite"
	"social-network/pkg/models"
)

func init() {
	if err := os.Chdir("../.."); err != nil {
		panic(err)
	}
}

func TestGroupAndEventModels(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}

	mustExec(`INSERT INTO users (email, password_hash, first_name, last_name, date_of_birth) VALUES (?, 'x', 'One', 'User', '2000-01-01')`, "one@test.com")
	mustExec(`INSERT INTO users (email, password_hash, first_name, last_name, date_of_birth) VALUES (?, 'x', 'Two', 'User', '2000-01-01')`, "two@test.com")

	groupID, err := models.CreateGroup(db, 1, "Test group", "Test description")
	if err != nil {
		t.Fatal(err)
	}
	member, err := models.IsMember(db, 1, groupID)
	if err != nil || !member {
		t.Fatalf("creator membership = %v, %v", member, err)
	}

	mustExec(`INSERT INTO follows (follower_id, following_id, status) VALUES (2, 1, 'accepted')`)
	if err := models.Invite(db, groupID, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := models.AcceptMember(db, groupID, 2); err != nil {
		t.Fatal(err)
	}

	eventID, err := models.CreateEvent(db, groupID, 1, "Meeting", "Bring ideas", "2026-12-01T18:30")
	if err != nil {
		t.Fatal(err)
	}
	if err := models.Respond(db, eventID, 2, "going"); err != nil {
		t.Fatal(err)
	}
	if err := models.Respond(db, eventID, 2, "not_going"); err != nil {
		t.Fatal(err)
	}

	events, err := models.EventsByGroup(db, groupID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].NotGoing != 1 || events[0].MyResponse == nil || *events[0].MyResponse != "not_going" {
		t.Fatalf("unexpected event: %+v", events)
	}

	if err := models.RequestJoin(db, groupID, 1); err == nil {
		t.Fatal("creator unexpectedly allowed a duplicate join request")
	}

	memberIDs, err := models.ConfirmedMemberIDs(db, groupID)
	if err != nil {
		t.Fatal(err)
	}
	if len(memberIDs) != 2 {
		t.Fatalf("confirmed members = %+v, want creator and accepted member", memberIDs)
	}
}
