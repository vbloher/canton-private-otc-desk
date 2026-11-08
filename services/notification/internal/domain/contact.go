package domain

import "time"

type ContactEvent struct {
	Email        string
	Unsubscribed bool
}

type ContactSync struct {
	Email         string
	Attempts      int
	NextAttemptAt time.Time
}

func (s ContactSync) Retry(now time.Time) ContactSync {
	s.Attempts++
	s.NextAttemptAt = now.Add(RetryDelay(s.Attempts))

	return s
}

func (s ContactSync) Exhausted() bool {
	return s.Attempts >= MaxAttempts
}
