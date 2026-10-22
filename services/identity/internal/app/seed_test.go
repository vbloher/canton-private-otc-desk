package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// seederFunc adapts a function to the houseSeeder the loop drives.
type seederFunc func(ctx context.Context) error

func (f seederFunc) SeedHouseMakers(ctx context.Context) error { return f(ctx) }

func Test_SeedHouseMakers_StopsAtTheFirstSuccess(t *testing.T) {
	var attempts atomic.Int32

	seedHouseMakers(t.Context(), seederFunc(func(context.Context) error {
		attempts.Add(1)

		return nil
	}), time.Millisecond)

	require.EqualValues(t, 1, attempts.Load())
}

func Test_SeedHouseMakers_RetriesUntilTheLedgerAnswers(t *testing.T) {
	var attempts atomic.Int32

	seedHouseMakers(t.Context(), seederFunc(func(context.Context) error {
		if attempts.Add(1) < 4 {
			return errors.New("ledger is not up yet")
		}

		return nil
	}), time.Millisecond)

	require.EqualValues(t, 4, attempts.Load(), "three failures, then the seed goes through")
}

func Test_SeedHouseMakers_WaitsBetweenAttempts(t *testing.T) {
	var times []time.Time

	seedHouseMakers(t.Context(), seederFunc(func(context.Context) error {
		times = append(times, time.Now())

		if len(times) < 3 {
			return errors.New("ledger is not up yet")
		}

		return nil
	}), 30*time.Millisecond)

	require.Len(t, times, 3)
	require.GreaterOrEqual(t, times[1].Sub(times[0]), 30*time.Millisecond)
	require.GreaterOrEqual(t, times[2].Sub(times[1]), 30*time.Millisecond)
}

func Test_SeedHouseMakers_StopsWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	var attempts atomic.Int32

	done := make(chan struct{})

	go func() {
		defer close(done)

		seedHouseMakers(ctx, seederFunc(func(context.Context) error {
			if attempts.Add(1) == 3 {
				cancel()
			}

			return errors.New("ledger is down")
		}), time.Millisecond)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the retry loop outlived its context")
	}

	require.EqualValues(t, 3, attempts.Load())
}

func Test_SeedHouseMakers_ContextEndedBeforeTheFirstAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var attempts atomic.Int32

	seedHouseMakers(ctx, seederFunc(func(context.Context) error {
		attempts.Add(1)

		return errors.New("canceled")
	}), time.Hour)

	require.LessOrEqual(t, attempts.Load(), int32(1))
}
