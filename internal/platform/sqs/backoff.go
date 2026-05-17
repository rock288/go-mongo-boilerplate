package sqs

import "time"

// SQSMaxDelay is the AWS hard limit for MessageDelaySeconds. backoffDelay
// will not return a value larger than this — values above the limit cause
// SendMessage to fail at runtime.
const SQSMaxDelay = 900 * time.Second

// backoffDelay returns base * 2^n clamped to [base, min(max, SQSMaxDelay)].
// Bit-shift overflow is prevented by capping the shift amount to 16, which
// already yields 65536x the base — well past any reasonable retry budget.
//
// Mirror of kafka.backoffDelay, with the SQS-specific 900s ceiling baked in.
func backoffDelay(base, max time.Duration, n int) time.Duration {
	if n < 0 {
		n = 0
	}
	if max > SQSMaxDelay {
		max = SQSMaxDelay
	}
	shift := n
	if shift > 16 {
		shift = 16
	}
	d := base * (1 << uint(shift)) //#nosec G115 -- shift clamped to [0,16] above
	if d <= 0 || d > max {
		return max
	}
	return d
}
