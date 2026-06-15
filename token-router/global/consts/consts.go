package consts

const (
	AppName = "token-router"

	// MaxAllocRetry caps how many full snapshot+CAS waves Alloc will attempt
	// before giving up. Bumped from 8 to 32 because every CAS failure means
	// some other goroutine made progress, so retrying is almost always the
	// right call. Bailing too eagerly produces spurious 429s under
	// contention and hurts the utilization KPI.
	MaxAllocRetry = 32
)
