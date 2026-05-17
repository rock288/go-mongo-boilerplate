package kafka

import (
	"testing"
	"time"
)

func TestBackoffDelay(t *testing.T) {
	t.Parallel()
	base := 100 * time.Millisecond
	max := 5 * time.Second
	tests := []struct {
		n    int
		want time.Duration
	}{
		{n: 0, want: 100 * time.Millisecond},
		{n: 1, want: 200 * time.Millisecond},
		{n: 2, want: 400 * time.Millisecond},
		{n: 3, want: 800 * time.Millisecond},
		{n: -1, want: 100 * time.Millisecond}, // negative clamped to 0
		{n: 100, want: 5 * time.Second},       // overflow clamped to max
	}
	for _, tt := range tests {
		got := backoffDelay(base, max, tt.n)
		if got != tt.want {
			t.Errorf("backoffDelay(%v, %v, %d) = %v, want %v", base, max, tt.n, got, tt.want)
		}
	}
}
