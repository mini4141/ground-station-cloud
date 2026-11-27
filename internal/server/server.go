// Package server implements the OpenAPI ServerInterface.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/mini4141/ground-station-cloud/internal/api"
	"github.com/mini4141/ground-station-cloud/internal/auth"
	"github.com/mini4141/ground-station-cloud/internal/db/sqlc"
	"github.com/mini4141/ground-station-cloud/internal/events"
	"github.com/mini4141/ground-station-cloud/internal/live"
	"github.com/mini4141/ground-station-cloud/internal/msg"
	"github.com/mini4141/ground-station-cloud/internal/orbit"
	"github.com/mini4141/ground-station-cloud/internal/schedule"
	"github.com/mini4141/ground-station-cloud/internal/simclock"
	"github.com/mini4141/ground-station-cloud/internal/storage"
)

var bookingsCreated = promauto.NewCounter(prometheus.CounterOpts{Name: "gs_bookings_created_total", Help: "Bookings accepted."})

// Server holds the dependencies of every handler.
type Server struct {
	Pool         *pgxpool.Pool
	Q            *sqlc.Queries
	Store        storage.Storage
	Live         live.Store
	Events       events.Sink
	Sched        events.Scheduler
	Dispatcher   *schedule.Dispatcher
	UploadSecret []byte
	PlanLeadMin  float64
}

var _ api.ServerInterface = (*Server)(nil)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) { writeJSON(w, code, api.Error{Error: msg}) }

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return false
	}
	return true
}

func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

func (s *Server) operator(ctx context.Context) (auth.Operator, error) {
	op := auth.From(ctx)
	return op, s.Q.UpsertOperator(ctx, sqlc.UpsertOperatorParams{ID: op.ID, Name: op.Name})
}

// GetHealth implements api.ServerInterface.
func (s *Server) GetHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.Health{Status: "ok"})
}

// GetSimClock implements api.ServerInterface.
func (s *Server) GetSimClock(w http.ResponseWriter, r *http.Request) {
	c, err := simclock.Load(r.Context(), s.Q)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, api.SimClock{SimTime: c.Now(), Scale: c.Scale})
}

// SetSimClock implements api.ServerInterface.
func (s *Server) SetSimClock(w http.ResponseWriter, r *http.Request) {
	var in api.SimClockInput
	if !decode(w, r, &in) {
		return
	}
	cur, err := simclock.Load(r.Context(), s.Q)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	scale := cur.Scale
	if in.Scale != nil && *in.Scale > 0 {
		scale = *in.Scale
	}
	if err := s.Q.SetSimClock(r.Context(), sqlc.SetSimClockParams{SimRef: in.SimTime, Scale: scale}); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.GetSimClock(w, r)
}

func toStation(x sqlc.Station) api.Station {
	return api.Station{Id: x.ID, Name: x.Name, Latitude: x.Lat, Longitude: x.Lon, AltitudeM: &x.AltM,
		MinElevationDeg: &x.MinElDeg, Band: x.Band, RatePerMinMinor: x.RatePerMinMinor}
}

// ListStations implements api.ServerInterface.
func (s *Server) ListStations(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Q.ListStations(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]api.Station, 0, len(rows))
	for _, x := range rows {
		out = append(out, toStation(x))
	}
	writeJSON(w, 200, out)
}

func deref(p *float64, def float64) float64 {
	if p == nil {
		return def
	}
	return *p
}

// CreateStation implements api.ServerInterface.
func (s *Server) CreateStation(w http.ResponseWriter, r *http.Request) {
	var in api.StationInput
	if !decode(w, r, &in) {
		return
	}
	x, err := s.Q.CreateStation(r.Context(), sqlc.CreateStationParams{Name: in.Name, Lat: in.Latitude, Lon: in.Longitude,
		AltM: deref(in.AltitudeM, 0), MinElDeg: deref(in.MinElevationDeg, 5), Band: in.Band, RatePerMinMinor: in.RatePerMinMinor})
	if isUnique(err) {
		writeErr(w, 409, "station name already exists")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, toStation(x))
}

// GetStation implements api.ServerInterface.
func (s *Server) GetStation(w http.ResponseWriter, r *http.Request, id string) {
	x, err := s.Q.GetStation(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 404, "station not found")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, toStation(x))
}

// UpdateStation implements api.ServerInterface.
func (s *Server) UpdateStation(w http.ResponseWriter, r *http.Request, id string) {
	var in api.StationPatch
	if !decode(w, r, &in) {
		return
	}
	x, err := s.Q.UpdateStation(r.Context(), sqlc.UpdateStationParams{ID: id, Lat: in.Latitude, Lon: in.Longitude, AltM: in.AltitudeM,
		MinElDeg: in.MinElevationDeg, Band: in.Band, RatePerMinMinor: in.RatePerMinMinor})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 404, "station not found")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, toStation(x))
}

// DeleteStation implements api.ServerInterface.
func (s *Server) DeleteStation(w http.ResponseWriter, r *http.Request, id string) {
	n, err := s.Q.DeleteStation(r.Context(), id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if n == 0 {
		writeErr(w, 404, "station not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetStationLive implements api.ServerInterface.
func (s *Server) GetStationLive(w http.ResponseWriter, r *http.Request, id string) {
	st, err := s.Q.GetStation(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 404, "station not found")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := api.LiveState{Station: st.Name, State: ptr(msg.StateOffline)}
	it, ok, err := s.Live.Get(r.Context(), st.Name)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if ok {
		if it.State != "" {
			out.State = &it.State
		}
		if !it.HeartbeatAt.IsZero() {
			out.HeartbeatAt = &it.HeartbeatAt
		}
		if len(it.Frame) > 0 {
			var t msg.Telemetry
			if json.Unmarshal(it.Frame, &t) == nil {
				out.Frame = &api.LiveFrame{T: t.T, BookingId: &t.BookingID, Az: t.Az, El: t.El, Snr: t.SNR, Lock: t.Lock, Bitrate: int32(t.Bitrate)}
			}
		}
	}
	writeJSON(w, 200, out)
}

func ptr[T any](v T) *T { return &v }

// ListSatellites implements api.ServerInterface.
func (s *Server) ListSatellites(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Q.ListSatellites(r.Context(), auth.From(r.Context()).ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]api.Satellite, 0, len(rows))
	for _, x := range rows {
		out = append(out, toSatellite(x))
	}
	writeJSON(w, 200, out)
}

func toSatellite(x sqlc.Satellite) api.Satellite {
	return api.Satellite{Id: x.ID, OperatorId: x.OperatorID, Name: x.Name, NoradId: x.NoradID, Tle1: x.Tle1, Tle2: x.Tle2}
}

// CreateSatellite implements api.ServerInterface.
func (s *Server) CreateSatellite(w http.ResponseWriter, r *http.Request) {
	var in api.SatelliteInput
	if !decode(w, r, &in) {
		return
	}
	if err := orbit.ValidateTLE(in.Tle1, in.Tle2); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	op, err := s.operator(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	x, err := s.Q.CreateSatellite(r.Context(), sqlc.CreateSatelliteParams{OperatorID: op.ID, Name: in.Name, NoradID: in.NoradId, Tle1: in.Tle1, Tle2: in.Tle2})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, toSatellite(x))
}

// ListPasses implements api.ServerInterface: runs SGP4 per station and caches predictions.
func (s *Server) ListPasses(w http.ResponseWriter, r *http.Request, p api.ListPassesParams) {
	ctx := r.Context()
	sat, err := s.Q.GetSatellite(ctx, sqlc.GetSatelliteParams{ID: p.Satellite, OperatorID: auth.From(ctx).ID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 404, "satellite not found")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	clk, err := simclock.Load(ctx, s.Q)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	from := clk.Now()
	if p.From != nil {
		from = *p.From
	}
	to := from.Add(24 * time.Hour)
	if p.To != nil {
		to = *p.To
	}
	if to.Sub(from) > 48*time.Hour {
		to = from.Add(48 * time.Hour)
	}
	stations, err := s.Q.ListStations(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := []api.Pass{}
	for _, st := range stations {
		if p.Station != nil && *p.Station != st.ID && *p.Station != st.Name {
			continue
		}
		preds, err := orbit.Passes(sat.Tle1, sat.Tle2, orbit.Station{LatDeg: st.Lat, LonDeg: st.Lon, AltM: st.AltM, MinElDeg: st.MinElDeg}, from, to)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		known, err := s.Q.ListPassesInRange(ctx, sqlc.ListPassesInRangeParams{SatelliteID: sat.ID, StationID: st.ID, FromT: from, ToT: to})
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		byAOS := map[int64]sqlc.Pass{}
		for _, k := range known {
			byAOS[k.Aos.Unix()] = k
		}
		for _, pr := range preds {
			row, ok := byAOS[pr.AOS.Unix()]
			if !ok {
				track := make([]msg.TrackPoint, 0, len(pr.Track))
				for _, pt := range pr.Track {
					track = append(track, msg.TrackPoint{T: pt.T, Az: round1(pt.AzDeg), El: round1(pt.ElDeg)})
				}
				tj, _ := json.Marshal(track)
				row, err = s.Q.UpsertPass(ctx, sqlc.UpsertPassParams{SatelliteID: sat.ID, StationID: st.ID, Aos: pr.AOS, Tca: pr.TCA, Los: pr.LOS, MaxEl: pr.MaxElDeg, Track: tj})
				if err != nil {
					writeErr(w, 500, err.Error())
					return
				}
			}
			var track []api.TrackPoint
			_ = json.Unmarshal(row.Track, &track)
			out = append(out, api.Pass{Id: row.ID, SatelliteId: sat.ID, StationId: st.ID, StationName: st.Name,
				Aos: row.Aos, Tca: row.Tca, Los: row.Los, MaxElevationDeg: row.MaxEl, Track: track})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Aos.Before(out[j].Aos) })
	writeJSON(w, 200, out)
}

func round1(f float64) float64 {
	v, _ := strconv.ParseFloat(strconv.FormatFloat(f, 'f', 1, 64), 64)
	return v
}

// CreateBooking implements api.ServerInterface: conflict-checked under a per-station advisory lock.
func (s *Server) CreateBooking(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in api.BookingInput
	if !decode(w, r, &in) {
		return
	}
	op, err := s.operator(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	pass, err := s.Q.GetPass(ctx, in.PassId)
	if err != nil || pass.OperatorID != op.ID {
		writeErr(w, 404, "pass not found")
		return
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := s.Q.WithTx(tx)
	if err := q.LockStation(ctx, pass.StationID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	wins, err := q.ListStationWindows(ctx, pass.StationID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	existing := make([]schedule.Window, 0, len(wins))
	for _, x := range wins {
		existing = append(existing, schedule.Window{AOS: x.Aos, LOS: x.Los})
	}
	if schedule.Conflicts(existing, schedule.Window{AOS: pass.Aos, LOS: pass.Los}) {
		writeErr(w, 409, "station already booked for an overlapping window")
		return
	}
	id, err := q.CreateBooking(ctx, sqlc.CreateBookingParams{PassID: pass.ID, OperatorID: op.ID})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	bookingsCreated.Inc()
	// arm the AOS-lead reminder (EventBridge Scheduler in AWS; the local ticker polls instead)
	if clk, err := simclock.Load(ctx, s.Q); err == nil {
		at := clk.RealAt(pass.Aos.Add(-time.Duration(s.PlanLeadMin * float64(time.Minute))))
		if err := s.Sched.Schedule(ctx, id, at); err != nil {
			writeErr(w, 500, "schedule: "+err.Error())
			return
		}
	}
	s.Events.Emit(ctx, "BookingCreated", map[string]any{"booking_id": id, "pass_id": pass.ID, "operator": op.ID})
	b, err := s.Q.GetBooking(ctx, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, bookingFromRow(b.ID, b.PassID, b.SatelliteID, b.SatelliteName, b.StationID, b.StationName, b.OperatorID, b.Status,
		b.Aos, b.Los, b.PlanSentAt, b.AosActual, b.LosActual, b.LockRatio, b.BytesDelivered))
}

func bookingFromRow(id, passID, satID, satName, stID, stName, opID, status string, aos, los time.Time, planSent, aosA, losA *time.Time, lock float64, bytes int64) api.Booking {
	return api.Booking{Id: id, PassId: passID, SatelliteId: satID, SatelliteName: satName, StationId: stID, StationName: stName, OperatorId: opID,
		Status: api.BookingStatus(status), Aos: aos, Los: los, PlanSentAt: planSent, AosActual: aosA, LosActual: losA, LockRatio: &lock, BytesDelivered: &bytes}
}

// ListBookings implements api.ServerInterface.
func (s *Server) ListBookings(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Q.ListBookings(r.Context(), auth.From(r.Context()).ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]api.Booking, 0, len(rows))
	for _, b := range rows {
		out = append(out, bookingFromRow(b.ID, b.PassID, b.SatelliteID, b.SatelliteName, b.StationID, b.StationName, b.OperatorID, b.Status,
			b.Aos, b.Los, b.PlanSentAt, b.AosActual, b.LosActual, b.LockRatio, b.BytesDelivered))
	}
	writeJSON(w, 200, out)
}

// GetBooking implements api.ServerInterface.
func (s *Server) GetBooking(w http.ResponseWriter, r *http.Request, id string) {
	b, err := s.Q.GetBooking(r.Context(), id)
	if err != nil || b.OperatorID != auth.From(r.Context()).ID {
		writeErr(w, 404, "booking not found")
		return
	}
	writeJSON(w, 200, bookingFromRow(b.ID, b.PassID, b.SatelliteID, b.SatelliteName, b.StationID, b.StationName, b.OperatorID, b.Status,
		b.Aos, b.Los, b.PlanSentAt, b.AosActual, b.LosActual, b.LockRatio, b.BytesDelivered))
}

// CreateUploadUrl implements api.ServerInterface; authorised by the plan's HMAC token.
func (s *Server) CreateUploadUrl(w http.ResponseWriter, r *http.Request, id string, p api.CreateUploadUrlParams) {
	if p.XUploadToken != schedule.UploadToken(s.UploadSecret, id) {
		writeErr(w, 401, "invalid upload token")
		return
	}
	key := "mission/" + id + ".bin"
	url, err := s.Store.PresignPut(r.Context(), key)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, api.UploadURL{Url: url, Method: "PUT", Key: key})
}

func slaRow(scope api.SlaRowScope, name string, passes, ok int32, ack, del float64) api.SlaRow {
	rate := 0.0
	if passes > 0 {
		rate = float64(ok) / float64(passes)
	}
	return api.SlaRow{Scope: scope, Name: name, Passes: passes, Successes: ok, SuccessRate: rate, AckLatencyP95Ms: &ack, DeliveryDelayP95Ms: &del}
}

// GetSla implements api.ServerInterface.
func (s *Server) GetSla(w http.ResponseWriter, r *http.Request) {
	ctx, op := r.Context(), auth.From(r.Context()).ID
	byStation, err := s.Q.SlaByStation(ctx, op)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	bySat, err := s.Q.SlaBySatellite(ctx, op)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := []api.SlaRow{}
	for _, x := range byStation {
		out = append(out, slaRow("station", x.Name, x.Passes, x.Successes, x.AckP95Ms, x.DeliveryP95Ms))
	}
	for _, x := range bySat {
		out = append(out, slaRow("satellite", x.Name, x.Passes, x.Successes, x.AckP95Ms, x.DeliveryP95Ms))
	}
	writeJSON(w, 200, out)
}

// GetBilling implements api.ServerInterface.
func (s *Server) GetBilling(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Q.Billing(r.Context(), auth.From(r.Context()).ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]api.BillingRow, 0, len(rows))
	for _, x := range rows {
		out = append(out, api.BillingRow{Month: x.Month, Station: x.Station, Minutes: x.Minutes, RatePerMinMinor: x.RatePerMinMinor, AmountMinor: x.AmountMinor})
	}
	writeJSON(w, 200, out)
}
