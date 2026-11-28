// Package wiring builds the dependencies shared by the cloud services from environment variables.
package wiring

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mini4141/ground-station-cloud/internal/db/sqlc"
	"github.com/mini4141/ground-station-cloud/internal/events"
	"github.com/mini4141/ground-station-cloud/internal/iot"
	"github.com/mini4141/ground-station-cloud/internal/mqtt"
	"github.com/mini4141/ground-station-cloud/internal/platform"
	"github.com/mini4141/ground-station-cloud/internal/schedule"
)

// Deps are the shared cloud-side dependencies.
type Deps struct {
	Pool       *pgxpool.Pool
	Q          *sqlc.Queries
	Pub        mqtt.Publisher
	Events     events.Sink
	Dispatcher *schedule.Dispatcher
}

// New connects Postgres, the plan publisher (PUBLISHER=mqtt|iot) and the event sink (EVENTS=log|eventbridge).
func New(ctx context.Context, clientID string) (*Deps, error) {
	pool, err := platform.OpenDB(ctx, platform.Env("DATABASE_URL", "postgres://gs:gs@localhost:5432/gs?sslmode=disable"))
	if err != nil {
		return nil, err
	}
	d := &Deps{Pool: pool, Q: sqlc.New(pool), Events: events.Log{}}
	switch platform.Env("PUBLISHER", "mqtt") {
	case "iot":
		if d.Pub, err = iot.New(ctx, platform.Env("IOT_ENDPOINT", ""), platform.Env("IOT_VPCE_HOST", "")); err != nil {
			return nil, err
		}
	case "mqtt":
		d.Pub, err = mqtt.Connect(mqtt.Options{URL: platform.Env("MQTT_URL", "ssl://localhost:8883"), ClientID: clientID + platform.Env("MQTT_CLIENT_SUFFIX", ""),
			CA: platform.Env("MQTT_CA", ".local/certs/ca.crt"), Cert: platform.Env("MQTT_CERT", ".local/certs/cloud.crt"), Key: platform.Env("MQTT_KEY", ".local/certs/cloud.key")})
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown PUBLISHER")
	}
	if platform.Env("EVENTS", "log") == "eventbridge" {
		if d.Events, err = events.NewBus(ctx); err != nil {
			return nil, err
		}
	}
	d.Dispatcher = &schedule.Dispatcher{Q: d.Q, Pub: d.Pub, Events: d.Events,
		UploadSecret: []byte(platform.Env("UPLOAD_SECRET", "dev-upload-secret")), APIBase: platform.Env("API_PUBLIC_URL", "http://localhost:8080")}
	slog.Info("dependencies ready", "publisher", platform.Env("PUBLISHER", "mqtt"))
	return d, nil
}
