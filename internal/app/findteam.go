package app

import (
	"strings"

	"arenasuite/internal/db"
)

// FindTeamFilter narrows the Find a Team lists. Empty fields are ignored.
type FindTeamFilter struct {
	CompetitionID int64
	Category      string
	Role          string // also matched against skills
}

// RecruitingTeams lists teams that are looking for members.
func (a *App) RecruitingTeams(f FindTeamFilter) ([]Team, error) {
	where := []string{"t.status = 'active'", "t.looking_for_members = 1"}
	var args []any
	if f.CompetitionID != 0 {
		where = append(where, "t.competition_id = ?")
		args = append(args, f.CompetitionID)
	}
	if f.Category != "" {
		where = append(where, "c.category = ?")
		args = append(args, f.Category)
	}
	if f.Role != "" {
		where = append(where, "(t.looking_for_role LIKE ? OR t.required_skills LIKE ?)")
		args = append(args, "%"+f.Role+"%", "%"+f.Role+"%")
	}
	return db.All[Team](a.DB, teamSelect+" WHERE "+strings.Join(where, " AND ")+" ORDER BY t.created_at DESC", args...)
}

const postSelect = `
	SELECT p.*, u.name AS user_name, u.profile_picture, c.name AS competition_name, c.category
	FROM looking_for_team_posts p
	JOIN users u ON u.id = p.user_id
	JOIN competitions c ON c.id = p.competition_id`

// OpenPosts lists "I need a team" posts.
func (a *App) OpenPosts(f FindTeamFilter) ([]Post, error) {
	where := []string{"p.status = 'open'"}
	var args []any
	if f.CompetitionID != 0 {
		where = append(where, "p.competition_id = ?")
		args = append(args, f.CompetitionID)
	}
	if f.Category != "" {
		where = append(where, "c.category = ?")
		args = append(args, f.Category)
	}
	if f.Role != "" {
		where = append(where, "(p.role LIKE ? OR p.skills LIKE ?)")
		args = append(args, "%"+f.Role+"%", "%"+f.Role+"%")
	}
	return db.All[Post](a.DB, postSelect+" WHERE "+strings.Join(where, " AND ")+" ORDER BY p.created_at DESC", args...)
}

type PostInput struct {
	CompetitionID                         int64
	Role, Skills, Experience, Description string
}

func (a *App) CreatePost(user *User, in PostInput) error {
	if in.CompetitionID == 0 || strings.TrimSpace(in.Role) == "" {
		return fail(400, "Pick a competition and the role you're looking for")
	}
	if in.Experience != "" {
		if err := mustBeOneOf(in.Experience, []string{"Beginner", "Intermediate", "Advanced"}, "experience"); err != nil {
			return err
		}
	}
	if _, err := a.getCompetition(a.DB, in.CompetitionID); err != nil {
		return err
	}
	_, err := a.DB.Exec(`
		INSERT INTO looking_for_team_posts (competition_id, user_id, role, skills, experience, description)
		VALUES (?, ?, ?, ?, ?, ?)`,
		in.CompetitionID, user.ID, in.Role, db.NullIfEmpty(in.Skills), db.NullIfEmpty(in.Experience), db.NullIfEmpty(in.Description))
	if db.IsUniqueViolation(err) {
		return fail(409, `You already have a "looking for a team" post for this competition`)
	}
	return err
}

func (a *App) DeletePost(user *User, postID int64) error {
	post, err := db.One[Post](a.DB, `SELECT * FROM looking_for_team_posts WHERE id = ?`, postID)
	if err != nil {
		return err
	}
	if post == nil {
		return fail(404, "Post not found")
	}
	if post.UserID != user.ID && !user.IsAdmin() {
		return fail(403, "You do not own this post")
	}
	_, err = a.DB.Exec(`DELETE FROM looking_for_team_posts WHERE id = ?`, postID)
	return err
}
