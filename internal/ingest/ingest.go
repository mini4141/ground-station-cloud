// Package ingest turns edge messages from the SQS event bus into DynamoDB live state and Postgres pass metrics.
package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/mini4141/ground-station-cloud/internal/db/sqlc"
	"github.com/mini4141/ground-station-cloud/internal/live"
	"github.com/mini4141/ground-station-cloud/internal/msg"
)

var processed = promauto.NewCounterVec(prometheus.CounterOpts{Name: "gs_ingest_messages_total", Help: "Edge messages ingested."}, []string{"kind"})

// Processor handles one bus message.
type Processor struct {
	Q    *sqlc.Queries
	Live live.Store
}

// Process decodes an enveloped edge message and applies it.
func (p *Processor) Process(ctx context.Context, body []byte) error {
	var env msg.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("envelope: %w", err)
	}
	processed.WithLabelValues(env.Kind).Inc()
	switch env.Kind {
	case "telemetry":
		var t msg.Telemetry
		if err := json.Unmarshal(body, &t); err != nil {
			return err
		}
		if err := p.Live.PutFrame(ctx, env.Station, body); err != nil {
			return err
		}
		if t.BookingID != "" {
			lock := int32(0)
			if t.Lock {
				lock = 1
			}
			return p.Q.RecordFrame(ctx, sqlc.RecordFrameParams{ID: t.BookingID, Locked: lock})
		}
	case "status":
		var s msg.Status
		if err := json.Unmarshal(body, &s); err != nil {
			return err
		}
		return p.Live.PutStatus(ctx, env.Station, s.State, s.Ts)
	case "events":
		var e msg.Event
		if err := json.Unmarshal(body, &e); err != nil {
			return err
		}
		slog.Info("edge event", "station", env.Station, "type", e.Type, "booking", e.BookingID)
		switch e.Type {
		case msg.EvPlanAck:
			return p.Q.RecordAck(ctx, sqlc.RecordAckParams{ID: e.BookingID, AckAt: e.Ts})
		case msg.EvAOS:
			return p.Q.RecordAOS(ctx, sqlc.RecordAOSParams{ID: e.BookingID, T: e.T})
		case msg.EvLOS:
			return p.Q.RecordLOS(ctx, sqlc.RecordLOSParams{ID: e.BookingID, T: e.T})
		case msg.EvUploadOK:
			return p.Q.RecordDelivery(ctx, sqlc.RecordDeliveryParams{ID: e.BookingID, Bytes: e.Bytes, DelayMs: int32(e.DelayMs)})
		}
	default:
		return fmt.Errorf("unknown kind %q", env.Kind)
	}
	return nil
}
