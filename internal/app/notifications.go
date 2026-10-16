package app

import "arenasuite/internal/db"

func (a *App) NotificationsFor(userID int64) ([]Notification, error) {
	return db.All[Notification](a.DB, `SELECT * FROM notifications WHERE user_id = ? ORDER BY created_at DESC, id DESC LIMIT 100`, userID)
}

func (a *App) UnreadCount(userID int64) int64 {
	n, _ := db.Count(a.DB, `SELECT COUNT(*) FROM notifications WHERE user_id = ? AND NOT is_read`, userID)
	return n
}

func (a *App) MarkRead(user *User, notificationID int64) error {
	n, err := db.One[Notification](a.DB, `SELECT * FROM notifications WHERE id = ?`, notificationID)
	if err != nil {
		return err
	}
	if n == nil {
		return fail(404, "Notification not found")
	}
	if n.UserID != user.ID {
		return fail(403, "This notification does not belong to you")
	}
	_, err = a.DB.Exec(`UPDATE notifications SET is_read = TRUE WHERE id = ?`, notificationID)
	return err
}

func (a *App) MarkAllRead(user *User) error {
	_, err := a.DB.Exec(`UPDATE notifications SET is_read = TRUE WHERE user_id = ? AND NOT is_read`, user.ID)
	return err
}
