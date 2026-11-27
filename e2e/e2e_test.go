//go:build e2e

// Package e2e runs a full simulated pass against the running local stack (scripts/dev-up.sh):
// book -> plan over MQTT -> agent slews rotator and locks modem -> telemetry -> mission file in storage -> SLA + billing.
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mini4141/ground-station-cloud/internal/api"
	"github.com/mini4141/ground-station-cloud/internal/mqtt"
	"github.com/mini4141/ground-station-cloud/internal/msg"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func token(t *testing.T) string {
	t.Helper()
	resp, err := http.Post(env("DEVOIDC_URL", "http://localhost:9000")+"/token", "application/json", strings.NewReader(`{"operator":"demo"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.AccessToken
}

func TestSimulatedPassEndToEnd(t *testing.T) {
	ctx := context.Background()
	tok := token(t)
	c, err := api.NewClientWithResponses(env("API_URL", "http://localhost:8080"), api.WithRequestEditorFn(func(_ context.Context, r *http.Request) error {
		r.Header.Set("Authorization", "Bearer "+tok)
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	// observe every MQTT message like `mosquitto_sub -t 'gs/#'`
	var mu sync.Mutex
	var frames, locked int
	var events []msg.Event
	var raw []string
	sub, err := mqtt.Connect(mqtt.Options{URL: env("MQTT_URL", "ssl://localhost:8883"), ClientID: "e2e-observer",
		CA: env("MQTT_CA", "../.local/certs/ca.crt"), Cert: env("MQTT_CERT", "../.local/certs/cloud.crt"), Key: env("MQTT_KEY", "../.local/certs/cloud.key")})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	if err := sub.Subscribe("gs/#", func(topic string, p []byte) {
		mu.Lock()
		defer mu.Unlock()
		raw = append(raw, time.Now().UTC().Format(time.RFC3339Nano)+" "+topic+" "+string(p))
		switch {
		case strings.HasSuffix(topic, "/telemetry"):
			var f msg.Telemetry
			if json.Unmarshal(p, &f) == nil && f.BookingID != "" {
				frames++
				if f.Lock {
					locked++
				}
			}
		case strings.HasSuffix(topic, "/events"):
			var e msg.Event
			if json.Unmarshal(p, &e) == nil {
				events = append(events, e)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}

	sats, _ := c.ListSatellitesWithResponse(ctx)
	var satID string
	for _, s := range *sats.JSON200 {
		if strings.HasPrefix(s.Name, "ISS") {
			satID = s.Id
		}
	}
	if satID == "" {
		t.Fatal("seed satellite ISS missing (run scripts/seed.sh)")
	}

	// pick the first Tokyo pass in a fixed window that is not booked yet
	from := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
	station := "tokyo"
	passes, err := c.ListPassesWithResponse(ctx, &api.ListPassesParams{Satellite: satID, Station: &station, From: &from, To: ptr(from.Add(24 * time.Hour))})
	if err != nil || passes.JSON200 == nil || len(*passes.JSON200) == 0 {
		t.Fatalf("no passes: %v", err)
	}
	bookings, _ := c.ListBookingsWithResponse(ctx)
	booked := map[string]bool{}
	for _, b := range *bookings.JSON200 {
		booked[b.PassId] = true
	}
	var pass *api.Pass
	for i, p := range *passes.JSON200 {
		if !booked[p.Id] {
			pass = &(*passes.JSON200)[i]
			break
		}
	}
	if pass == nil {
		t.Fatal("all passes already booked; reset the stack")
	}

	// jump simulated time to 8 minutes before AOS (plan goes out at AOS-5min)
	if _, err := c.SetSimClockWithResponse(ctx, api.SetSimClockJSONRequestBody{SimTime: pass.Aos.Add(-8 * time.Minute), Scale: ptr(20.0)}); err != nil {
		t.Fatal(err)
	}
	bk, err := c.CreateBookingWithResponse(ctx, api.BookingInput{PassId: pass.Id})
	if err != nil || bk.JSON201 == nil {
		t.Fatalf("booking failed: %v %d", err, bk.StatusCode())
	}
	id := bk.JSON201.Id

	// the same pass again conflicts
	dup, _ := c.CreateBookingWithResponse(ctx, api.BookingInput{PassId: pass.Id})
	if dup.StatusCode() != http.StatusConflict {
		t.Fatalf("overlapping booking: want 409, got %d", dup.StatusCode())
	}

	// wait for the agent to deliver the mission file
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := false
		for _, e := range events {
			if e.BookingID == id && e.Type == msg.EvUploadOK {
				done = true
			}
		}
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(time.Second)
	}
	time.Sleep(3 * time.Second) // let ingest drain the bus

	mu.Lock()
	defer mu.Unlock()
	if p := env("E2E_MQTT_LOG", ""); p != "" {
		_ = os.WriteFile(p, []byte(strings.Join(raw, "\n")+"\n"), 0o644)
	}
	seen := map[string]bool{}
	for _, e := range events {
		if e.BookingID == id {
			seen[e.Type] = true
		}
	}
	for _, want := range []string{msg.EvPlanAck, msg.EvPrepos, msg.EvAOS, msg.EvLOS, msg.EvUploadOK} {
		if !seen[want] {
			t.Fatalf("event %s missing; saw %v", want, seen)
		}
	}
	if frames < 5 || float64(locked)/float64(frames) < 0.8 {
		t.Fatalf("telemetry: %d frames, %d locked", frames, locked)
	}
	obj := filepath.Join(env("UPLOAD_DIR", "../.local/uploads"), "mission", id+".bin")
	if st, err := os.Stat(obj); err != nil || st.Size() == 0 {
		t.Fatalf("mission file not in storage: %v", err)
	}

	got, _ := c.GetBookingWithResponse(ctx, id)
	if got.JSON200 == nil || got.JSON200.Status != api.Completed || *got.JSON200.LockRatio < 0.8 {
		t.Fatalf("booking not completed: %+v", got.JSON200)
	}
	sla, _ := c.GetSlaWithResponse(ctx)
	if sla.JSON200 == nil || len(*sla.JSON200) == 0 || (*sla.JSON200)[0].Passes < 1 {
		t.Fatalf("sla empty: %+v", sla.JSON200)
	}
	bill, _ := c.GetBillingWithResponse(ctx)
	if bill.JSON200 == nil || len(*bill.JSON200) == 0 || (*bill.JSON200)[0].Minutes <= 0 {
		t.Fatalf("billing empty: %+v", bill.JSON200)
	}
	live, _ := c.GetStationLiveWithResponse(ctx, pass.StationId)
	if live.JSON200 == nil || live.JSON200.Frame == nil {
		t.Fatalf("no live frame in DynamoDB: %+v", live.JSON200)
	}
	t.Logf("pass ok: %d frames (%d locked), file %s, sla=%d rows, billing=%.1f min", frames, locked, obj, len(*sla.JSON200), (*bill.JSON200)[0].Minutes)
}

func ptr[T any](v T) *T { return &v }
