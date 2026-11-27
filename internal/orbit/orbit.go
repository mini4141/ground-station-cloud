// Package orbit predicts satellite passes over ground stations with SGP4.
package orbit

import (
	"errors"
	"math"
	"time"

	sat "github.com/joshuaferrara/go-satellite"
)

// Station is an observer location.
type Station struct {
	LatDeg, LonDeg, AltM, MinElDeg float64
}

// Point is one az/el sample of a pass track.
type Point struct {
	T     time.Time
	AzDeg float64
	ElDeg float64
}

// Pass is one visibility window.
type Pass struct {
	AOS, TCA, LOS time.Time
	MaxElDeg      float64
	Track         []Point // sampled every TrackStep
}

const (
	// TrackStep is the az/el sampling interval of a pass track.
	TrackStep  = 10 * time.Second
	coarseStep = 30 * time.Second
)

// ValidateTLE guards go-satellite, which aborts the process on malformed input.
func ValidateTLE(l1, l2 string) error {
	for i, l := range []string{l1, l2} {
		if len(l) != 69 {
			return errors.New("tle lines must be 69 characters")
		}
		if l[0] != byte('1'+i) {
			return errors.New("tle line numbers must be 1 and 2")
		}
		sum := 0
		for _, c := range l[:68] {
			switch {
			case c >= '0' && c <= '9':
				sum += int(c - '0')
			case c == '-':
				sum++
			}
		}
		if sum%10 != int(l[68]-'0') {
			return errors.New("tle checksum mismatch")
		}
	}
	return nil
}

// LookAngle returns azimuth/elevation in degrees of the satellite seen from st at t.
func LookAngle(s sat.Satellite, st Station, t time.Time) (az, el float64) {
	t = t.UTC()
	pos, _ := sat.Propagate(s, t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second())
	jd := sat.JDay(t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second())
	obs := sat.LatLong{Latitude: st.LatDeg * math.Pi / 180, Longitude: st.LonDeg * math.Pi / 180}
	la := sat.ECIToLookAngles(pos, obs, st.AltM/1000, jd)
	return la.Az * 180 / math.Pi, la.El * 180 / math.Pi
}

// Passes returns all passes in [from, to] with elevation above the station minimum.
func Passes(tle1, tle2 string, st Station, from, to time.Time) ([]Pass, error) {
	if err := ValidateTLE(tle1, tle2); err != nil {
		return nil, err
	}
	s := sat.TLEToSat(tle1, tle2, sat.GravityWGS84)
	el := func(t time.Time) float64 { _, e := LookAngle(s, st, t); return e }
	// refine finds the crossing of min elevation between a (below/above) and b to 1 s.
	refine := func(a, b time.Time, aboveAtB bool) time.Time {
		for b.Sub(a) > time.Second {
			m := a.Add(b.Sub(a) / 2)
			if (el(m) >= st.MinElDeg) == aboveAtB {
				b = m
			} else {
				a = m
			}
		}
		return b
	}

	var out []Pass
	from = from.UTC().Truncate(time.Second)
	prev, prevT := el(from), from
	var aos time.Time
	inPass := prev >= st.MinElDeg
	if inPass {
		aos = from
	}
	for t := from.Add(coarseStep); !t.After(to); t = t.Add(coarseStep) {
		cur := el(t)
		switch {
		case !inPass && cur >= st.MinElDeg:
			aos, inPass = refine(prevT, t, true), true
		case inPass && cur < st.MinElDeg:
			los := refine(prevT, t, false)
			out = append(out, build(s, st, aos, los))
			inPass = false
		}
		prev, prevT = cur, t
	}
	_ = prev
	return out, nil
}

func build(s sat.Satellite, st Station, aos, los time.Time) Pass {
	p := Pass{AOS: aos, LOS: los, TCA: aos}
	for t := aos; ; t = t.Add(TrackStep) {
		if t.After(los) {
			t = los
		}
		az, e := LookAngle(s, st, t)
		p.Track = append(p.Track, Point{T: t, AzDeg: az, ElDeg: e})
		if e > p.MaxElDeg {
			p.MaxElDeg, p.TCA = e, t
		}
		if t.Equal(los) {
			break
		}
	}
	return p
}
