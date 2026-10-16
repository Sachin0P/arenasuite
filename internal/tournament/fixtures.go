// Package tournament holds the pure tournament maths: building fixtures and
// ranking standings. Nothing here touches the database, so it's easy to test.
package tournament

import (
	"fmt"
	"math/rand/v2"
)

// Fixture is one match to be created. Team ids of 0 mean "empty slot".
type Fixture struct {
	Round      string
	RoundOrder int
	TeamA      int64
	TeamB      int64
	Winner     int64 // set up front only for byes (one team, no opponent)

	// For knockouts: which match in the next round the winner goes to.
	HasNext   bool
	NextRound int    // index into the rounds slice
	NextIndex int    // index of the match inside that round
	NextSlot  string // "A" or "B"
}

// SeedOrder returns the standard bracket order for a power-of-two size,
// e.g. 8 -> [1 8 4 5 2 7 3 6]. Going from N seeds to 2N, each seed s becomes
// the pair (s, 2N+1-s). This spreads byes across different first-round
// matches so no match ends up with two empty slots.
func SeedOrder(size int) []int {
	seeds := []int{1}
	for len(seeds) < size {
		n := len(seeds) * 2
		next := make([]int, 0, n)
		for _, s := range seeds {
			next = append(next, s, n+1-s)
		}
		seeds = next
	}
	return seeds
}

// Knockout builds a single-elimination bracket. Teams are shuffled first.
// Returns one slice of fixtures per round, first round first.
func Knockout(teamIDs []int64) [][]Fixture {
	teams := append([]int64(nil), teamIDs...)
	rand.Shuffle(len(teams), func(i, j int) { teams[i], teams[j] = teams[j], teams[i] })
	return knockoutFromSeeds(teams)
}

// knockoutFromSeeds builds the bracket with teams already in seed order
// (teams[0] is seed 1). Split out from Knockout so tests are deterministic.
func knockoutFromSeeds(teams []int64) [][]Fixture {
	// Round the bracket size up to the next power of two.
	size := 2
	for size < len(teams) {
		size *= 2
	}
	totalRounds := 0
	for s := size; s > 1; s /= 2 {
		totalRounds++
	}

	// Slot i holds seed SeedOrder[i]. Seeds beyond the team count are byes (0),
	// so the top seeds are the ones who get a free pass.
	slots := make([]int64, size)
	for i, seed := range SeedOrder(size) {
		if seed <= len(teams) {
			slots[i] = teams[seed-1]
		}
	}

	var rounds [][]Fixture
	current := slots
	for roundIndex := 0; len(current) > 1; roundIndex++ {
		matchCount := len(current) / 2
		name := roundName(matchCount, totalRounds, roundIndex)
		round := make([]Fixture, matchCount)
		next := make([]int64, matchCount)

		for i := range round {
			a, b := current[i*2], current[i*2+1]
			f := Fixture{Round: name, RoundOrder: roundIndex, TeamA: a, TeamB: b}
			// A bye: the only team present goes straight through.
			if a != 0 && b == 0 {
				f.Winner = a
			} else if a == 0 && b != 0 {
				f.Winner = b
			}
			round[i] = f
			next[i] = f.Winner // known winners (byes) already fill the next round
		}
		rounds = append(rounds, round)
		current = next
	}

	// Link every match (except the final) to the slot its winner feeds.
	for r := 0; r < len(rounds)-1; r++ {
		for i := range rounds[r] {
			rounds[r][i].HasNext = true
			rounds[r][i].NextRound = r + 1
			rounds[r][i].NextIndex = i / 2
			if i%2 == 0 {
				rounds[r][i].NextSlot = "A"
			} else {
				rounds[r][i].NextSlot = "B"
			}
		}
	}
	return rounds
}

func roundName(matchesInRound, totalRounds, roundIndex int) string {
	switch matchesInRound {
	case 1:
		return "Final"
	case 2:
		return "Semifinal"
	case 4:
		return "Quarterfinal"
	}
	return fmt.Sprintf("Round %d of %d", roundIndex+1, totalRounds)
}

// RoundRobin uses the circle method: keep the first team fixed and rotate
// the others one step each round, so every team meets every other team once.
// An odd number of teams gets a bye slot (0); matches against it are skipped.
func RoundRobin(teamIDs []int64) [][]Fixture {
	teams := append([]int64(nil), teamIDs...)
	if len(teams)%2 != 0 {
		teams = append(teams, 0)
	}
	n := len(teams)

	var rounds [][]Fixture
	for round := 0; round < n-1; round++ {
		var matches []Fixture
		for i := 0; i < n/2; i++ {
			a, b := teams[i], teams[n-1-i]
			if a != 0 && b != 0 {
				matches = append(matches, Fixture{
					Round:      fmt.Sprintf("Round %d", round+1),
					RoundOrder: round,
					TeamA:      a,
					TeamB:      b,
				})
			}
		}
		rounds = append(rounds, matches)

		// Rotate: move the last team to position 1, shifting the rest right.
		last := teams[n-1]
		copy(teams[2:], teams[1:n-1])
		teams[1] = last
	}
	return rounds
}

// IsKnockout reports whether a competition format uses a bracket.
func IsKnockout(format string) bool {
	switch format {
	case "knockout", "single_elimination", "group_knockout", "double_elimination":
		return true
	}
	return false
}
