package db

import "testing"

func TestRebind(t *testing.T) {
	got := rebind(`SELECT * FROM users WHERE id = ? AND name = 'a?b' AND email = ?`)
	want := `SELECT * FROM users WHERE id = $1 AND name = 'a?b' AND email = $2`
	if got != want {
		t.Fatalf("rebind:\n got %s\nwant %s", got, want)
	}
}
