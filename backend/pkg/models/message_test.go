// Owner: B
package models

import (
	"errors"
	"testing"
)

// Uses the helpers from post_test.go (openPostTestDB, postTestUser, ...).

func TestCanChat(t *testing.T) {
	db := openPostTestDB(t)
	a := postTestUser(t, db, "a")
	b := postTestUser(t, db, "b")
	c := postTestUser(t, db, "c")
	d := postTestUser(t, db, "d")
	postTestFollow(t, db, a, b, "accepted") // a follows b
	postTestFollow(t, db, c, a, "accepted") // c follows a
	postTestFollow(t, db, d, a, "pending")  // d only asked

	for _, tc := range []struct {
		name        string
		user, other int64
		want        bool
	}{
		{"I follow them", a, b, true},
		{"they follow me (the other direction)", b, a, true},
		{"they follow me, second case", a, c, true},
		{"nobody follows", b, c, false},
		{"pending request does not count", a, d, false},
		{"pending request, other direction", d, a, false},
		{"myself", a, a, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CanChat(db, tc.user, tc.other)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("CanChat = %v, want %v", got, tc.want)
			}
		})
	}

	if _, err := CanChat(db, a, 9999); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("unknown user: got %v, want ErrUserNotFound", err)
	}
}

func TestSavePrivateMessage(t *testing.T) {
	db := openPostTestDB(t)
	a := postTestUser(t, db, "a")
	b := postTestUser(t, db, "b")
	stranger := postTestUser(t, db, "stranger")
	postTestFollow(t, db, a, b, "accepted")

	m, err := SavePrivateMessage(db, b, a, "Hi 😀") // b writes to a: allowed, a follows b
	if err != nil {
		t.Fatal(err)
	}
	if m.SenderID != b || m.ReceiverID != a || m.Content != "Hi 😀" || m.IsRead || m.CreatedAt.IsZero() || m.ID == 0 {
		t.Errorf("unexpected message: %+v", m)
	}

	if _, err := SavePrivateMessage(db, stranger, a, "hello"); !errors.Is(err, ErrChatNotAllowed) {
		t.Errorf("stranger: got %v, want ErrChatNotAllowed", err)
	}
	if _, err := SavePrivateMessage(db, a, 9999, "hello"); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("unknown receiver: got %v, want ErrUserNotFound", err)
	}
	if _, err := SavePrivateMessage(db, a, a, "hello"); !errors.Is(err, ErrChatNotAllowed) {
		t.Errorf("to myself: got %v, want ErrChatNotAllowed", err)
	}

	var n int
	db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&n)
	if n != 1 {
		t.Errorf("%d messages stored, want only the allowed one", n)
	}
}

func TestPrivateHistory(t *testing.T) {
	db := openPostTestDB(t)
	a := postTestUser(t, db, "a")
	b := postTestUser(t, db, "b")
	c := postTestUser(t, db, "c")
	stranger := postTestUser(t, db, "stranger")
	postTestFollow(t, db, a, b, "accepted")
	postTestFollow(t, db, a, c, "accepted")

	send := func(from, to int64, text string) {
		if _, err := SavePrivateMessage(db, from, to, text); err != nil {
			t.Fatal(err)
		}
	}
	send(a, b, "1")
	send(b, a, "2")
	send(a, c, "other conversation")
	send(a, b, "3")
	send(b, a, "4")
	send(a, b, "5")
	// a group message by a must never appear in a private conversation
	postTestExec(t, db, `INSERT INTO groups (creator_id, title, description) VALUES (?, 'g', 'd')`, a)
	postTestExec(t, db, `INSERT INTO messages (sender_id, group_id, content) VALUES (?, 1, 'group talk')`, a)

	texts := func(ms []ChatMessage) string {
		s := ""
		for _, m := range ms {
			s += m.Content + ","
		}
		return s
	}

	all, more, err := PrivateHistory(db, a, b, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if texts(all) != "1,2,3,4,5," || more {
		t.Errorf("history = %s more=%v, want 1..5 oldest first", texts(all), more)
	}
	// the same conversation seen from the other side
	if fromB, _, _ := PrivateHistory(db, b, a, 0, 0); texts(fromB) != "1,2,3,4,5," {
		t.Errorf("seen from b: %s", texts(fromB))
	}

	// paging: the newest 2, then the 2 before them, then the last one
	p1, more, _ := PrivateHistory(db, a, b, 2, 0)
	if texts(p1) != "4,5," || !more {
		t.Errorf("page 1 = %s more=%v", texts(p1), more)
	}
	p2, more, _ := PrivateHistory(db, a, b, 2, p1[0].ID)
	if texts(p2) != "2,3," || !more {
		t.Errorf("page 2 = %s more=%v", texts(p2), more)
	}
	p3, more, _ := PrivateHistory(db, a, b, 2, p2[0].ID)
	if texts(p3) != "1," || more {
		t.Errorf("page 3 = %s more=%v", texts(p3), more)
	}

	// no conversation yet: [] not nil
	quiet := postTestUser(t, db, "quiet")
	postTestFollow(t, db, a, quiet, "accepted")
	empty, _, err := PrivateHistory(db, a, quiet, 0, 0)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("empty page = %#v, %v", empty, err)
	}

	if _, _, err := PrivateHistory(db, stranger, a, 0, 0); !errors.Is(err, ErrChatNotAllowed) {
		t.Errorf("stranger: got %v, want ErrChatNotAllowed", err)
	}
	if _, _, err := PrivateHistory(db, a, 9999, 0, 0); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("unknown user: got %v, want ErrUserNotFound", err)
	}
}

func TestUnreadAndMarkRead(t *testing.T) {
	db := openPostTestDB(t)
	me := postTestUser(t, db, "me")
	b := postTestUser(t, db, "b")
	c := postTestUser(t, db, "c")
	stranger := postTestUser(t, db, "stranger")
	postTestFollow(t, db, me, b, "accepted")
	postTestFollow(t, db, c, me, "accepted")

	for _, s := range []struct {
		from, to int64
	}{{b, me}, {b, me}, {c, me}, {me, b}} { // the one I wrote is not "unread" for me
		if _, err := SavePrivateMessage(db, s.from, s.to, "x"); err != nil {
			t.Fatal(err)
		}
	}
	// a group message must not count either
	postTestExec(t, db, `INSERT INTO groups (creator_id, title, description) VALUES (?, 'g', 'd')`, b)
	postTestExec(t, db, `INSERT INTO messages (sender_id, group_id, content) VALUES (?, 1, 'group talk')`, b)

	count := func() int {
		n, err := UnreadPrivateCount(db, me)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(); n != 3 {
		t.Fatalf("unread = %d, want 3", n)
	}

	if err := MarkConversationRead(db, me, b); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 1 { // only c's message is left
		t.Errorf("unread after reading b = %d, want 1", n)
	}
	// reading b's conversation did not touch what I sent to b
	if n, _ := UnreadPrivateCount(db, b); n != 1 {
		t.Errorf("b's unread = %d, want 1 (my message to b)", n)
	}

	if err := MarkConversationRead(db, me, stranger); !errors.Is(err, ErrChatNotAllowed) {
		t.Errorf("stranger: got %v, want ErrChatNotAllowed", err)
	}
	if err := MarkConversationRead(db, me, 9999); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("unknown user: got %v, want ErrUserNotFound", err)
	}
}

func TestChatContacts(t *testing.T) {
	db := openPostTestDB(t)
	me := postTestUser(t, db, "me")
	follower := postTestUser(t, db, "follower") // follows me
	followee := postTestUser(t, db, "followee") // I follow them
	quiet := postTestUser(t, db, "quiet")       // related, no messages yet
	postTestUser(t, db, "stranger")             // not related: must not be listed
	pending := postTestUser(t, db, "pending")   // only a request: must not be listed
	postTestFollow(t, db, follower, me, "accepted")
	postTestFollow(t, db, me, followee, "accepted")
	postTestFollow(t, db, quiet, me, "accepted")
	postTestFollow(t, db, pending, me, "pending")

	// older conversation with the follower (2 unread for me), newer one with the followee
	SavePrivateMessage(db, follower, me, "f1")
	SavePrivateMessage(db, follower, me, "f2")
	SavePrivateMessage(db, me, followee, "to followee")

	got, err := ChatContacts(db, me)
	if err != nil {
		t.Fatal(err)
	}
	var order []int64
	for _, c := range got {
		order = append(order, c.User.ID)
	}
	want := []int64{followee, follower, quiet} // newest conversation first, no-conversation last
	if len(order) != 3 || order[0] != want[0] || order[1] != want[1] || order[2] != want[2] {
		t.Fatalf("contacts = %v, want %v", order, want)
	}

	if lm := got[0].LastMessage; lm == nil || lm.Content != "to followee" || got[0].UnreadCount != 0 {
		t.Errorf("followee: %+v", got[0])
	}
	if lm := got[1].LastMessage; lm == nil || lm.Content != "f2" || got[1].UnreadCount != 2 {
		t.Errorf("follower: last=%+v unread=%d, want f2 and 2", got[1].LastMessage, got[1].UnreadCount)
	}
	if got[2].LastMessage != nil || got[2].UnreadCount != 0 || got[2].User.FirstName != "quiet" {
		t.Errorf("quiet: %+v", got[2])
	}

	// nobody to talk to: [] not nil
	loner := postTestUser(t, db, "loner")
	none, err := ChatContacts(db, loner)
	if err != nil || none == nil || len(none) != 0 {
		t.Errorf("loner = %#v, %v", none, err)
	}
}

func TestGroupChatMembershipAndHistory(t *testing.T) {
	db := openPostTestDB(t)
	member := postTestUser(t, db, "member")
	outsider := postTestUser(t, db, "outsider")

	postTestExec(t, db, `INSERT INTO groups (creator_id, title, description) VALUES (?, 'g', 'd')`, member)
	postTestExec(t, db, `INSERT INTO group_members (group_id, user_id, status) VALUES (1, ?, 'member')`, member)

	message, err := SaveGroupMessage(db, member, 1, "hello group")
	if err != nil {
		t.Fatal(err)
	}
	if message.GroupID != 1 || message.SenderID != member || message.Content != "hello group" {
		t.Fatalf("unexpected group message: %+v", message)
	}

	messages, hasMore, err := GroupHistory(db, member, 1, 20, 0)
	if err != nil || hasMore || len(messages) != 1 {
		t.Fatalf("member history: messages=%d more=%v err=%v", len(messages), hasMore, err)
	}

	if _, err := SaveGroupMessage(db, outsider, 1, "secret"); !errors.Is(err, ErrGroupChatNotAllowed) {
		t.Fatalf("outsider save error = %v, want ErrGroupChatNotAllowed", err)
	}
	if _, _, err := GroupHistory(db, outsider, 1, 20, 0); !errors.Is(err, ErrGroupChatNotAllowed) {
		t.Fatalf("outsider history error = %v, want ErrGroupChatNotAllowed", err)
	}
}
