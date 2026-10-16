package tournament

import (
	"reflect"
	"testing"
)

func TestSeedOrder(t *testing.T) {
	got := SeedOrder(8)
	want := []int{1, 8, 4, 5, 2, 7, 3, 6}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SeedOrder(8) = %v, want %v", got, want)
	}
}

func TestKnockoutByesNeverMeet(t *testing.T) {
	// 5 teams -> bracket of 8 -> 3 byes, each in a different first-round match.
	rounds := knockoutFromSeeds([]int64{1, 2, 3, 4, 5})
	if len(rounds) != 3 {
		t.Fatalf("expected 3 rounds, got %d", len(rounds))
	}
	for _, m := range rounds[0] {
		if m.TeamA == 0 && m.TeamB == 0 {
			t.Fatalf("first-round match with two byes: %+v", m)
		}
	}
	if rounds[1][0].Round != "Semifinal" || rounds[2][0].Round != "Final" {
		t.Fatalf("unexpected round names: %s, %s", rounds[1][0].Round, rounds[2][0].Round)
	}
	if rounds[2][0].HasNext {
		t.Fatal("the final should not feed another match")
	}
	// Seed 1 has a bye, so it should already be waiting in the semifinal.
	if rounds[1][0].TeamA != 1 {
		t.Fatalf("seed 1 should advance on a bye, got %+v", rounds[1][0])
	}
}

func TestRoundRobinEveryPairOnce(t *testing.T) {
	teams := []int64{1, 2, 3, 4, 5}
	seen := map[[2]int64]int{}
	for _, round := range RoundRobin(teams) {
		for _, m := range round {
			a, b := m.TeamA, m.TeamB
			if a > b {
				a, b = b, a
			}
			seen[[2]int64{a, b}]++
		}
	}
	if len(seen) != 10 { // 5 choose 2
		t.Fatalf("expected 10 unique pairings, got %d", len(seen))
	}
	for pair, n := range seen {
		if n != 1 {
			t.Fatalf("pair %v played %d times", pair, n)
		}
	}
}

func TestStandings(t *testing.T) {
	rows := Standings([]int64{1, 2, 3}, []Result{
		{TeamA: 1, TeamB: 2, ScoreA: 3, ScoreB: 1, Winner: 1},
		{TeamA: 2, TeamB: 3, ScoreA: 2, ScoreB: 2, Winner: 0},
	})
	if rows[0].TeamID != 1 || rows[0].Points != 3 || rows[0].Rank != 1 {
		t.Fatalf("team 1 should lead with 3 points: %+v", rows[0])
	}
	// Teams 2 and 3 both have 1 point; team 3 wins on score difference (0 vs -2).
	if rows[1].TeamID != 3 || rows[2].TeamID != 2 {
		t.Fatalf("unexpected order: %+v", rows)
	}
}
