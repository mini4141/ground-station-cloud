package schedule

import (
	"testing"
	"time"
)

func TestConflicts(t *testing.T) {
	t0 := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	booked := []Window{{AOS: t0, LOS: t0.Add(10 * time.Minute)}}
	if !Conflicts(booked, Window{AOS: t0.Add(5 * time.Minute), LOS: t0.Add(15 * time.Minute)}) {
		t.Fatal("overlapping window must conflict")
	}
	if Conflicts(booked, Window{AOS: t0.Add(10 * time.Minute), LOS: t0.Add(20 * time.Minute)}) {
		t.Fatal("back-to-back windows must not conflict")
	}
}
