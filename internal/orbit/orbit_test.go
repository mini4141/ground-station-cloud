package orbit

import (
	"os"
	"strings"
	"testing"
	"time"
)

func readTLE(t *testing.T, file string) (string, string) {
	t.Helper()
	b, err := os.ReadFile("../../data/tle/" + file)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(string(b), "\r", "")), "\n")
	return strings.TrimRight(lines[1], " "), strings.TrimRight(lines[2], " ")
}

// A fixed TLE, station and window must give the same passes every time.
func TestPassesTokyoDeterministic(t *testing.T) {
	l1, l2 := readTLE(t, "iss.tle")
	tokyo := Station{LatDeg: 35.68, LonDeg: 139.69, AltM: 40, MinElDeg: 5}
	from := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
	got, err := Passes(l1, l2, tokyo, from, from.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 3 || len(got) > 8 {
		t.Fatalf("expected a handful of LEO passes in 24h, got %d", len(got))
	}
	again, _ := Passes(l1, l2, tokyo, from, from.Add(24*time.Hour))
	for i, p := range got {
		if !p.AOS.Equal(again[i].AOS) || p.MaxElDeg != again[i].MaxElDeg {
			t.Fatalf("pass %d not deterministic", i)
		}
		if !p.AOS.Before(p.TCA) && !p.AOS.Equal(p.TCA) || !p.TCA.Before(p.LOS) {
			t.Fatalf("pass %d ordering wrong: %v %v %v", i, p.AOS, p.TCA, p.LOS)
		}
		if d := p.LOS.Sub(p.AOS); d < time.Minute || d > 16*time.Minute {
			t.Fatalf("pass %d implausible duration %v", i, d)
		}
		if p.MaxElDeg < 5 || p.MaxElDeg > 90 {
			t.Fatalf("pass %d max elevation %v", i, p.MaxElDeg)
		}
	}
	first := got[0]
	want := time.Date(2026, 11, 27, 9, 59, 5, 0, time.UTC)
	if d := first.AOS.Sub(want); d < -2*time.Second || d > 2*time.Second || first.MaxElDeg < 83 || first.MaxElDeg > 83.6 {
		t.Fatalf("golden first pass drifted: %v %.2f", first.AOS, first.MaxElDeg)
	}
	t.Logf("first pass AOS=%s LOS=%s maxEl=%.2f points=%d", first.AOS.Format(time.RFC3339), first.LOS.Format(time.RFC3339), first.MaxElDeg, len(first.Track))
}

func TestValidateTLERejectsGarbage(t *testing.T) {
	if ValidateTLE("1 bad", "2 bad") == nil {
		t.Fatal("expected error")
	}
}
