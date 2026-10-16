package tournament

import "sort"

const (
	PointsWin  = 3
	PointsDraw = 1
	PointsLoss = 0
)

// Result is a completed match, as far as the standings care.
type Result struct {
	TeamA, TeamB   int64
	ScoreA, ScoreB int64
	Winner         int64 // 0 means a draw
}

// Row is one line of the standings table.
type Row struct {
	TeamID          int64
	Played          int64
	Won             int64
	Lost            int64
	Draw            int64
	Points          int64
	ScoreDifference int64
	Rank            int64
}

// Standings replays every completed match from zero and ranks the teams by
// points, then score difference, then wins. teamIDs are the approved teams,
// so teams that haven't played yet still appear with zeros.
func Standings(teamIDs []int64, results []Result) []Row {
	rows := map[int64]*Row{}
	var order []int64 // keep insertion order so ties stay stable
	get := func(id int64) *Row {
		if r, ok := rows[id]; ok {
			return r
		}
		r := &Row{TeamID: id}
		rows[id] = r
		order = append(order, id)
		return r
	}
	for _, id := range teamIDs {
		get(id)
	}

	for _, m := range results {
		a, b := get(m.TeamA), get(m.TeamB)
		a.Played++
		b.Played++
		a.ScoreDifference += m.ScoreA - m.ScoreB
		b.ScoreDifference += m.ScoreB - m.ScoreA

		switch m.Winner {
		case m.TeamA:
			a.Won++
			a.Points += PointsWin
			b.Lost++
			b.Points += PointsLoss
		case m.TeamB:
			b.Won++
			b.Points += PointsWin
			a.Lost++
			a.Points += PointsLoss
		default:
			a.Draw++
			a.Points += PointsDraw
			b.Draw++
			b.Points += PointsDraw
		}
	}

	ranked := make([]Row, 0, len(order))
	for _, id := range order {
		ranked = append(ranked, *rows[id])
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		x, y := ranked[i], ranked[j]
		if x.Points != y.Points {
			return x.Points > y.Points
		}
		if x.ScoreDifference != y.ScoreDifference {
			return x.ScoreDifference > y.ScoreDifference
		}
		return x.Won > y.Won
	})
	for i := range ranked {
		ranked[i].Rank = int64(i + 1)
	}
	return ranked
}
