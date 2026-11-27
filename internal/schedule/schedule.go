// Package schedule holds booking conflict detection and pass-plan dispatch.
package schedule

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/mini4141/ground-station-cloud/internal/db/sqlc"
	"github.com/mini4141/ground-station-cloud/internal/events"
	"github.com/mini4141/ground-station-cloud/internal/mqtt"
	"github.com/mini4141/ground-station-cloud/internal/msg"
	"github.com/mini4141/ground-station-cloud/internal/simclock"
)

// Window is a booked time span on one station.
type Window struct{ AOS, LOS time.Time }

// Conflicts reports whether cand overlaps any existing window (touching ends do not conflict).
func Conflicts(existing []Window, cand Window) bool {
	for _, w := range existing {
		if cand.AOS.Before(w.LOS) && cand.LOS.After(w.AOS) {
			return true
		}
	}
	return false
}

var plansDispatched = promauto.NewCounter(prometheus.CounterOpts{Name: "gs_pass_plans_dispatched_total", Help: "Pass plans published to stations."})

// UploadToken authorises an agent to request an upload URL for one booking.
func UploadToken(secret []byte, bookingID string) string {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(bookingID))
	return hex.EncodeToString(m.Sum(nil))
}

var bandFreq = map[string]int64{"UHF": 437_500_000, "S": 2_200_000_000, "X": 8_200_000_000}

// Dispatcher publishes pass plans.
type Dispatcher struct {
	Q            *sqlc.Queries
	Pub          mqtt.Publisher
	Events       events.Sink
	UploadSecret []byte
	APIBase      string // base URL the agent uses to request upload URLs
}

// Dispatch sends the plan for a booking once (idempotent).
func (d *Dispatcher) Dispatch(ctx context.Context, bookingID string) error {
	b, err := d.Q.GetBooking(ctx, bookingID)
	if err != nil {
		return fmt.Errorf("get booking: %w", err)
	}
	pass, err := d.Q.GetPass(ctx, b.PassID)
	if err != nil {
		return err
	}
	st, err := d.Q.GetStation(ctx, b.StationID)
	if err != nil {
		return err
	}
	clk, err := simclock.Load(ctx, d.Q)
	if err != nil {
		return err
	}
	n, err := d.Q.MarkDispatched(ctx, bookingID)
	if err != nil || n == 0 {
		return err // already dispatched elsewhere
	}
	var track []msg.TrackPoint
	if err := json.Unmarshal(pass.Track, &track); err != nil {
		return err
	}
	plan := msg.Plan{
		SchemaVersion: msg.SchemaVersion, Type: "plan", BookingID: bookingID, Station: st.Name, Satellite: b.SatelliteName,
		AOS: b.Aos, LOS: b.Los, FreqHz: bandFreq[st.Band], Mode: "GMSK9600", Track: track,
		SimNow: clk.Now(), SimScale: clk.Scale, APIBase: d.APIBase,
		UploadToken: UploadToken(d.UploadSecret, bookingID), SentAt: time.Now().UTC(),
	}
	body, _ := json.Marshal(plan)
	if err := d.Pub.Publish(ctx, msg.Topic(st.Name, "cmd"), body); err != nil {
		_ = d.Q.RevertDispatch(ctx, bookingID)
		return fmt.Errorf("publish plan: %w", err)
	}
	plansDispatched.Inc()
	slog.Info("pass plan dispatched", "booking", bookingID, "station", st.Name, "aos", b.Aos)
	d.Events.Emit(ctx, "PlanDispatched", map[string]any{"booking_id": bookingID, "station": st.Name, "aos": b.Aos})
	return nil
}
