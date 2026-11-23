// Owner: C
// CreateEvent, EventsByGroup, Respond

package models

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Event describes an event and the current user's RSVP state.
type Event struct {
	ID          int64     `json:"id"`
	GroupID     int64     `json:"group_id"`
	CreatorID   int64     `json:"creator_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	EventTime   time.Time `json:"event_time"`
	CreatedAt   time.Time `json:"created_at"`
	Going       int       `json:"going_count"`
	NotGoing    int       `json:"not_going_count"`
	MyResponse  *string   `json:"my_response,omitempty"`
}

// formatEventTime accepts time.Time or a date/time string commonly sent by
// datetime-local in the browser.
func formatEventTime(value any) (string, error) {
	switch eventTime := value.(type) {
	case time.Time:
		return eventTime.Format("2006-01-02 15:04:05"), nil
	case string:
		if eventTime == "" {
			return "", errors.New("event_time is required")
		}
		return eventTime, nil
	default:
		return "", fmt.Errorf("event_time must be a string or time.Time")
	}
}

// CreateEvent inserts an event in a group.
func CreateEvent(db *sql.DB, groupID, creatorID int64, title, description string, eventTime any) (int64, error) {
	formatted, err := formatEventTime(eventTime)
	if err != nil {
		return 0, err
	}
	if title == "" || description == "" {
		return 0, errors.New("title and description are required")
	}

	result, err := db.Exec(`INSERT INTO events
		(group_id, creator_id, title, description, event_time)
		VALUES (?, ?, ?, ?, ?)`, groupID, creatorID, title, description, formatted)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// EventsByGroup lists events and RSVP counts. viewerID is optional, but
// passing it is necessary to populate my_response.
func EventsByGroup(db *sql.DB, groupID int64, viewerID ...int64) ([]Event, error) {
	var viewer int64
	if len(viewerID) > 0 {
		viewer = viewerID[0]
	}

	rows, err := db.Query(`
		SELECT e.id, e.group_id, e.creator_id, e.title, e.description,
		       e.event_time, e.created_at,
		       (SELECT COUNT(*) FROM event_responses r
		        WHERE r.event_id = e.id AND r.response = 'going'),
		       (SELECT COUNT(*) FROM event_responses r
		        WHERE r.event_id = e.id AND r.response = 'not_going'),
		       (SELECT r.response FROM event_responses r
		        WHERE r.event_id = e.id AND r.user_id = ?)
		FROM events e
		WHERE e.group_id = ?
		ORDER BY e.event_time ASC, e.id ASC`, viewer, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]Event, 0)
	for rows.Next() {
		var event Event
		var response sql.NullString
		if err := rows.Scan(&event.ID, &event.GroupID, &event.CreatorID,
			&event.Title, &event.Description, &event.EventTime, &event.CreatedAt,
			&event.Going, &event.NotGoing, &response); err != nil {
			return nil, err
		}
		if response.Valid {
			event.MyResponse = &response.String
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// Respond inserts or switches a member's RSVP for an event.
func Respond(db *sql.DB, eventID, userID int64, response string) error {
	if response != "going" && response != "not_going" {
		return errors.New("response must be going or not_going")
	}
	_, err := db.Exec(`INSERT INTO event_responses (event_id, user_id, response)
		VALUES (?, ?, ?)
		ON CONFLICT(event_id, user_id) DO UPDATE SET
			response = excluded.response,
			updated_at = CURRENT_TIMESTAMP`, eventID, userID, response)
	return err
}
