//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"

	"github.com/mini4141/ground-station-cloud/internal/api"
	"github.com/mini4141/ground-station-cloud/internal/auth"
	"github.com/mini4141/ground-station-cloud/internal/db/sqlc"
	"github.com/mini4141/ground-station-cloud/internal/events"
	"github.com/mini4141/ground-station-cloud/internal/live"
	"github.com/mini4141/ground-station-cloud/internal/msg"
	"github.com/mini4141/ground-station-cloud/internal/platform"
	"github.com/mini4141/ground-station-cloud/internal/schedule"
	"github.com/mini4141/ground-station-cloud/internal/server"
	"github.com/mini4141/ground-station-cloud/internal/storage"
)

type fakePub struct {
	mu   sync.Mutex
	msgs map[string][][]byte
}

func (f *fakePub) Publish(_ context.Context, topic string, p []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.msgs == nil {
		f.msgs = map[string][][]byte{}
	}
	f.msgs[topic] = append(f.msgs[topic], p)
	return nil
}

func freshDB(t *testing.T) string {
	t.Helper()
	base := os.Getenv("DATABASE_URL")
	if base == "" {
		base = "postgres://gs:gs@localhost:5432/gs?sslmode=disable"
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("gs_it_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, _ := pgx.Connect(ctx, base)
		defer c.Close(ctx)
		_, _ = c.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
	})
	return strings.Replace(base, "/gs?", "/"+name+"?", 1)
}

func TestCreateStationComputePassesBook(t *testing.T) {
	ctx := context.Background()
	pool, err := platform.OpenDB(ctx, freshDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := sqlc.New(pool)
	pub := &fakePub{}
	srv := &server.Server{Pool: pool, Q: q, Store: &storage.Local{Dir: t.TempDir(), Secret: "s", PublicBase: "http://x"}, Live: &live.Memory{},
		Events: events.Log{}, Sched: events.Ticker{}, UploadSecret: []byte("s"), PlanLeadMin: 5}
	srv.Dispatcher = &schedule.Dispatcher{Q: q, Pub: pub, Events: events.Log{}, UploadSecret: []byte("s"), APIBase: "http://x"}
	ts := httptest.NewServer(srv.Handler(&auth.Verifier{TrustGateway: true}, nil, ""))
	defer ts.Close()

	tok, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": "op-test", "email": "t@example.test"}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	do := func(method, path string, body any, out any) int {
		t.Helper()
		var rd *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		} else {
			rd = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, ts.URL+path, rd)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if out != nil {
			_ = json.NewDecoder(resp.Body).Decode(out)
		}
		return resp.StatusCode
	}

	// 1. create station
	var st api.Station
	alt, minEl := 40.0, 5.0
	if c := do("POST", "/stations", api.StationInput{Name: "tokyo", Latitude: 35.68, Longitude: 139.69, AltitudeM: &alt, MinElevationDeg: &minEl, Band: "S", RatePerMinMinor: 120}, &st); c != 201 {
		t.Fatalf("create station: %d", c)
	}
	// 2. satellite from the bundled TLE
	tleBytes, _ := os.ReadFile("../../data/tle/iss.tle")
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(string(tleBytes), "\r", "")), "\n")
	var sat api.Satellite
	if c := do("POST", "/satellites", api.SatelliteInput{Name: "ISS", NoradId: 25544, Tle1: strings.TrimRight(lines[1], " "), Tle2: strings.TrimRight(lines[2], " ")}, &sat); c != 201 {
		t.Fatalf("create satellite: %d", c)
	}
	// 3. compute passes
	var passes []api.Pass
	q1 := fmt.Sprintf("/passes?satellite=%s&from=2026-11-27T09:00:00Z&to=2026-11-28T09:00:00Z", sat.Id)
	if c := do("GET", q1, nil, &passes); c != 200 || len(passes) < 3 {
		t.Fatalf("passes: %d (%d)", c, len(passes))
	}
	// 4. book, then the overlapping duplicate conflicts
	var bk api.Booking
	if c := do("POST", "/bookings", api.BookingInput{PassId: passes[0].Id}, &bk); c != 201 {
		t.Fatalf("book: %d", c)
	}
	if c := do("POST", "/bookings", api.BookingInput{PassId: passes[0].Id}, nil); c != 409 {
		t.Fatalf("overlap: want 409 got %d", c)
	}
	// 5. dispatching publishes the plan on gs/tokyo/cmd
	if err := srv.Dispatcher.Dispatch(ctx, bk.Id); err != nil {
		t.Fatal(err)
	}
	raw := pub.msgs["gs/tokyo/cmd"]
	if len(raw) != 1 {
		t.Fatalf("plan messages: %d", len(raw))
	}
	var plan msg.Plan
	_ = json.Unmarshal(raw[0], &plan)
	if plan.BookingID != bk.Id || plan.SchemaVersion != 1 || len(plan.Track) < 10 || plan.FreqHz == 0 {
		t.Fatalf("bad plan: %+v", plan)
	}
}
