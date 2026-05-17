package sqs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBackoffDelay(t *testing.T) {
	t.Parallel()

	const base = time.Second
	const max = 5 * time.Minute

	cases := []struct {
		name string
		n    int
		want time.Duration
	}{
		{"n=0 → base", 0, base},
		{"n=1 → 2*base", 1, 2 * base},
		{"n=2 → 4*base", 2, 4 * base},
		{"n=3 → 8*base", 3, 8 * base},
		{"n=8 → 256*base (under max)", 8, 256 * base},
		{"n=10 → max (1024s > 300s)", 10, max},
		{"n=100 → max (clamp shift)", 100, max},
		{"n=-1 → base (treated as 0)", -1, base},
	}
	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := backoffDelay(base, max, tt.n)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBackoffDelay_ClampsToSQSMax(t *testing.T) {
	t.Parallel()
	// User configures max above SQS limit — must clamp silently.
	got := backoffDelay(time.Second, 30*time.Minute, 20)
	assert.Equal(t, SQSMaxDelay, got)
}
