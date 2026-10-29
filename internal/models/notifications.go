package models

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Notification has JSON tags because new notifications are also pushed to
// the browser as JSON over the live-updates stream.
type Notification struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"userId"`
	Type        string    `json:"type"`
	Message     string    `json:"message"`
	ReferenceID int64     `json:"referenceId"`
	IsRead      bool      `json:"isRead"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Link is the page where the user can act on the notification, or "" if
// there is none. ReferenceID is a competition id for registration types and
// a team id for team types.
func (n Notification) Link() string {
	if n.ReferenceID == 0 {
		return ""
	}

	switch n.Type {
	case "registrationSubmitted":
		return fmt.Sprintf("/organizer/competitions/%d/manage#registrations", n.ReferenceID)
	case "registrationApproved", "registrationRejected":
		return fmt.Sprintf("/competitions/%d", n.ReferenceID)
	case "teamJoinRequest", "teamInvitationResponse":
		return fmt.Sprintf("/teams/%d", n.ReferenceID)
	case "teamInvitation":
		return "/dashboard" // invitations are accepted from the dashboard
	}

	return ""
}

type NotificationModel struct {
	DB *sql.DB
}

const notificationColumns = `id, user_id, type, message, COALESCE(reference_id, 0), is_read, created_at`

func scanNotification(row scanner) (*Notification, error) {
	n := &Notification{}
	err := row.Scan(&n.ID, &n.UserID, &n.Type, &n.Message, &n.ReferenceID, &n.IsRead, &n.CreatedAt)
	return n, err
}

// Insert saves a notification and returns the full row, ready to be pushed
// to the browser.
func (m *NotificationModel) Insert(userID int64, kind, message string, referenceID int64) (*Notification, error) {
	stmt := `INSERT INTO notifications (user_id, type, message, reference_id)
	VALUES ($1, $2, $3, NULLIF($4, 0))
	RETURNING ` + notificationColumns

	return scanNotification(m.DB.QueryRow(stmt, userID, kind, message, referenceID))
}

func (m *NotificationModel) Get(id int64) (*Notification, error) {
	n, err := scanNotification(m.DB.QueryRow(`SELECT `+notificationColumns+` FROM notifications WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}

	return n, nil
}

// ForUser returns a user's 100 most recent notifications.
func (m *NotificationModel) ForUser(userID int64) ([]Notification, error) {
	stmt := `SELECT ` + notificationColumns + ` FROM notifications
	WHERE user_id = $1
	ORDER BY created_at DESC, id DESC
	LIMIT 100`

	rows, err := m.DB.Query(stmt, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []Notification

	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		notifications = append(notifications, *n)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return notifications, nil
}

func (m *NotificationModel) UnreadCount(userID int64) (int, error) {
	var n int

	err := m.DB.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND NOT is_read`, userID).Scan(&n)
	return n, err
}

func (m *NotificationModel) MarkRead(id int64) error {
	return execOne(m.DB, `UPDATE notifications SET is_read = TRUE WHERE id = $1`, id)
}

func (m *NotificationModel) MarkAllRead(userID int64) error {
	_, err := m.DB.Exec(`UPDATE notifications SET is_read = TRUE WHERE user_id = $1 AND NOT is_read`, userID)
	return err
}
