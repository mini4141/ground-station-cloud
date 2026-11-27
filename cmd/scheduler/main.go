// Command scheduler dispatches pass plans at AOS minus the lead time.
// In AWS an EventBridge Scheduler one-time schedule invokes it per booking; locally it polls against sim time.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/mini4141/ground-station-cloud/internal/db/sqlc"
	"github.com/mini4141/ground-station-cloud/internal/platform"
	"github.com/mini4141/ground-station-cloud/internal/simclock"
	"github.com/mini4141/ground-station-cloud/internal/wiring"
)

func main() {
	ctx := context.Background()
	defer platform.Setup(ctx, "scheduler")()
	d, err := wiring.New(ctx, "cloud-scheduler")
	if err != nil {
		slog.Error("scheduler failed", "err", err)
		os.Exit(1)
	}
	if platform.InLambda() {
		lambda.Start(func(ctx context.Context, in struct {
			BookingID string `json:"booking_id"`
		}) error {
			return d.Dispatcher.Dispatch(ctx, in.BookingID)
		})
		return
	}
	platform.ServeAdmin(platform.Env("ADMIN_ADDR", ":8092"))
	lead := platform.EnvFloat("PLAN_LEAD_MIN", 5) * 60
	for range time.Tick(time.Second) {
		tick(ctx, d, lead)
	}
}

func tick(ctx context.Context, d *wiring.Deps, leadSecs float64) {
	clk, err := simclock.Load(ctx, d.Q)
	if err != nil {
		slog.Error("clock", "err", err)
		return
	}
	ids, err := d.Q.DueBookings(ctx, sqlc.DueBookingsParams{LeadSecs: leadSecs, SimNow: clk.Now()})
	if err != nil {
		slog.Error("due bookings", "err", err)
		return
	}
	for _, id := range ids {
		if err := d.Dispatcher.Dispatch(ctx, id); err != nil {
			slog.Error("dispatch", "booking", id, "err", err)
		}
	}
}
