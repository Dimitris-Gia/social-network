// Owner: A
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"social-network/pkg/models"
)

// notifications decodes GET /api/notifications for the given user.
func (e *followEnv) notifications(cookie *http.Cookie) []models.Notification {
	e.t.Helper()
	var list []models.Notification
	body := e.expect("GET", "/api/notifications", "", cookie, 200)
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		e.t.Fatalf("decode %s: %v", body, err)
	}
	return list
}

func TestFollowRequestNotificationLifecycle(t *testing.T) {
	e := newFollowEnv(t)
	alice, ca := e.user("alice@x.com", false)
	bob, cb := e.user("bob@x.com", true) // private
	_, cc := e.user("carol@x.com", false)

	// Following a PUBLIC profile creates no request notification.
	e.expect("POST", fmt.Sprintf("/api/users/%d/follow", alice), "", cc, 200)
	if list := e.notifications(ca); len(list) != 0 {
		t.Fatalf("public follow should not notify: %+v", list)
	}

	// Private: bob gets a follow_request from alice, unread.
	e.expect("POST", fmt.Sprintf("/api/users/%d/follow", bob), "", ca, 200)
	list := e.notifications(cb)
	if len(list) != 1 || list[0].Type != models.NotifFollowRequest || list[0].IsRead ||
		list[0].Actor == nil || list[0].Actor.ID != alice || list[0].Group != nil {
		t.Fatalf("bad follow_request notification: %+v", list)
	}
	if body := e.expect("GET", "/api/notifications/unread-count", "", cb, 200); body != `{"count":1}` {
		t.Fatalf("unread count: %s", body)
	}
	if list := e.notifications(ca); len(list) != 0 {
		t.Fatal("the requester must not see the target's notification")
	}

	// Carol can't mark bob's notification as read: 404, as if it didn't exist.
	readPath := fmt.Sprintf("/api/notifications/%d/read", list[0].ID)
	e.expect("POST", readPath, "", cc, 404)
	e.expect("POST", readPath, "", cb, 204)
	if body := e.expect("GET", "/api/notifications/unread-count", "", cb, 200); body != `{"count":0}` {
		t.Fatalf("after read: %s", body)
	}
	e.expect("POST", "/api/notifications/abc/read", "", cb, 400)

	// Accepting from the bell (same endpoint) removes the notification.
	e.expect("POST", fmt.Sprintf("/api/follow-requests/%d/accept", alice), "", cb, 204)
	if list := e.notifications(cb); len(list) != 0 {
		t.Fatalf("answered request still in the bell: %+v", list)
	}

	// Cancelling a pending request removes it too.
	e.expect("DELETE", fmt.Sprintf("/api/users/%d/follow", bob), "", ca, 204)
	e.expect("POST", fmt.Sprintf("/api/users/%d/follow", bob), "", ca, 200)
	e.expect("DELETE", fmt.Sprintf("/api/users/%d/follow", bob), "", ca, 204)
	if list := e.notifications(cb); len(list) != 0 {
		t.Fatalf("cancelled request still in the bell: %+v", list)
	}

	// Going public auto-accepts and clears every request.
	e.expect("POST", fmt.Sprintf("/api/users/%d/follow", bob), "", ca, 200)
	e.expect("POST", fmt.Sprintf("/api/users/%d/follow", bob), "", cc, 200)
	if n := len(e.notifications(cb)); n != 2 {
		t.Fatalf("want 2 requests, got %d", n)
	}
	e.expect("PATCH", "/api/me/privacy", `{"is_private":false}`, cb, 200)
	if list := e.notifications(cb); len(list) != 0 {
		t.Fatalf("requests left after going public: %+v", list)
	}
}

func TestGroupNotificationShapeAndReadAll(t *testing.T) {
	e := newFollowEnv(t)
	alice, _ := e.user("alice@x.com", false)
	bob, cb := e.user("bob@x.com", false)

	// What C will do: an invite and an event notification with group/event.
	res, err := e.db.Exec(`INSERT INTO groups (creator_id, title, description) VALUES (?, 'Go learners', 'd')`, alice)
	if err != nil {
		t.Fatal(err)
	}
	groupID, _ := res.LastInsertId()
	res, err = e.db.Exec(`INSERT INTO events (group_id, creator_id, title, description, event_time)
		VALUES (?, ?, 'Meetup', 'd', '2026-12-01 18:00:00')`, groupID, alice)
	if err != nil {
		t.Fatal(err)
	}
	eventID, _ := res.LastInsertId()

	h := &Handler{DB: e.db} // no hub: notify must still work
	h.notify(models.NewNotification{UserID: bob, ActorID: alice, Type: models.NotifGroupInvite, GroupID: groupID})
	h.notify(models.NewNotification{UserID: bob, ActorID: alice, Type: models.NotifGroupEvent, GroupID: groupID, EventID: eventID})

	list := e.notifications(cb)
	if len(list) != 2 || list[0].Type != models.NotifGroupEvent { // newest first
		t.Fatalf("bad list: %+v", list)
	}
	if list[0].Event == nil || list[0].Event.Title != "Meetup" || list[0].Group == nil ||
		list[0].Group.Title != "Go learners" || list[1].Event != nil {
		t.Fatalf("group/event not joined: %+v", list)
	}

	e.expect("POST", "/api/notifications/read-all", "", cb, 204)
	if body := e.expect("GET", "/api/notifications/unread-count", "", cb, 200); body != `{"count":0}` {
		t.Fatalf("read-all: %s", body)
	}

	// C deletes an answered invite with DeleteNotifications.
	if err := models.DeleteNotifications(e.db, bob, models.NotifGroupInvite, 0, groupID); err != nil {
		t.Fatal(err)
	}
	if list := e.notifications(cb); len(list) != 1 || list[0].Type != models.NotifGroupEvent {
		t.Fatalf("only the invite should be gone: %+v", list)
	}

	// Never leaks the actor's email.
	if body := e.expect("GET", "/api/notifications", "", cb, 200); strings.Contains(body, "@x.com") {
		t.Fatalf("notification leaks an email: %s", body)
	}
}
