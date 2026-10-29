package main

import (
	"testing"
	"time"

	"arenasuite/internal/assert"
)

func TestHumanDate(t *testing.T) {
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}

	// humanDate formats in time.Local (the site's timezone), so pin it for
	// the test and restore it afterwards.
	original := time.Local
	time.Local = ist
	t.Cleanup(func() { time.Local = original })

	// Create a slice of anonymous structs containing the test case name,
	// input to our humanDate() function (the tm field), and expected output
	// (the want field).
	tests := []struct {
		name string
		tm   time.Time
		want string
	}{
		{
			name: "IST",
			tm:   time.Date(2026, 10, 20, 10, 15, 0, 0, ist),
			want: "Oct 20, 2026",
		},
		{
			name: "Empty",
			tm:   time.Time{},
			want: "",
		},
		{
			// 20:00 UTC is already the next day in India.
			name: "UTC",
			tm:   time.Date(2026, 10, 20, 20, 0, 0, 0, time.UTC),
			want: "Oct 21, 2026",
		},
	}

	// Loop over the test cases.
	for _, tt := range tests {
		// Use the t.Run() function to run a sub-test for each test case. The
		// first parameter to this is the name of the test (which is used to
		// identify the sub-test in any log output) and the second parameter is
		// an anonymous function containing the actual test for each case.
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, humanDate(tt.tm), tt.want)
		})
	}
}

// TestNewTemplateCache checks that every page parses together with the base
// template and partials.
func TestNewTemplateCache(t *testing.T) {
	cache, err := newTemplateCache()
	assert.NilError(t, err)

	for _, page := range []string{"home.tmpl", "competition.tmpl", "competition-form.tmpl", "error.tmpl"} {
		_, ok := cache[page]
		assert.Equal(t, ok, true)
	}
}
