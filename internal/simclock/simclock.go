// Package simclock maps wall-clock time to accelerated simulated time.
package simclock

import (
	"context"
	"time"

	"github.com/mini4141/ground-station-cloud/internal/db/sqlc"
)

// Clock maps real time to simulated time: sim = SimRef + (real-RealRef)*Scale.
type Clock struct {
	RealRef, SimRef time.Time
	Scale           float64
}

// Now returns the current simulated time.
func (c Clock) Now() time.Time { return c.At(time.Now()) }

// At converts a real instant to simulated time.
func (c Clock) At(real time.Time) time.Time {
	return c.SimRef.Add(time.Duration(float64(real.Sub(c.RealRef)) * c.Scale))
}

// RealAt converts a simulated instant to wall-clock time.
func (c Clock) RealAt(sim time.Time) time.Time {
	return c.RealRef.Add(time.Duration(float64(sim.Sub(c.SimRef)) / c.Scale))
}

// Load reads the shared clock row.
func Load(ctx context.Context, q *sqlc.Queries) (Clock, error) {
	r, err := q.GetSimClock(ctx)
	if err != nil {
		return Clock{}, err
	}
	return Clock{RealRef: r.RealRef, SimRef: r.SimRef, Scale: r.Scale}, nil
}
