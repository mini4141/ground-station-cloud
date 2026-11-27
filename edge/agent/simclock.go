package main

import (
	"time"

	"github.com/mini4141/ground-station-cloud/internal/msg"
)

// simClock derives simulated time from the plan's simNow/simScale anchor.
type simClock struct {
	simRef, realRef time.Time
	scale           float64
}

func newSimClock(simNow time.Time, scale float64) simClock {
	if scale <= 0 {
		scale = 1
	}
	return simClock{simRef: simNow, realRef: time.Now(), scale: scale}
}

// Now returns the current simulated time.
func (c simClock) Now() time.Time {
	return c.simRef.Add(time.Duration(float64(time.Since(c.realRef)) * c.scale)).UTC()
}

// SleepUntil blocks until simulated time reaches t.
func (c simClock) SleepUntil(t time.Time) {
	if d := time.Duration(float64(t.Sub(c.Now())) / c.scale); d > 0 {
		time.Sleep(d)
	}
}

// interpolate returns the az/el of the predicted track at simulated time t.
func interpolate(track []msg.TrackPoint, t time.Time) (az, el float64) {
	if t.Before(track[0].T) {
		return track[0].Az, track[0].El
	}
	for i := 1; i < len(track); i++ {
		if t.Before(track[i].T) {
			a, b := track[i-1], track[i]
			f := float64(t.Sub(a.T)) / float64(b.T.Sub(a.T))
			return a.Az + (b.Az-a.Az)*f, a.El + (b.El-a.El)*f
		}
	}
	last := track[len(track)-1]
	return last.Az, last.El
}
