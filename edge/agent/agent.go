package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/mini4141/ground-station-cloud/internal/mqtt"
	"github.com/mini4141/ground-station-cloud/internal/msg"
)

// Agent executes pass plans against the station hardware.
type Agent struct {
	Station string
	Rot     Rotator
	Mod     Modem
	MQ      *mqtt.Client
	Busy    chan struct{} // one pass at a time
	state   string
}

func (a *Agent) publish(kind string, v any, retained bool) {
	b, _ := json.Marshal(v)
	if err := a.MQ.PublishRetained(msg.Topic(a.Station, kind), b, retained); err != nil {
		slog.Error("publish", "kind", kind, "err", err)
	}
}

func (a *Agent) setState(s string) {
	a.state = s
	a.publish("status", msg.Status{SchemaVersion: msg.SchemaVersion, Station: a.Station, State: s, Ts: time.Now().UTC()}, true)
}

func (a *Agent) heartbeat(ctx context.Context) {
	a.setState(msg.StateIdle)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
			a.setState(a.state)
		}
	}
}

func (a *Agent) event(p *msg.Plan, clk simClock, typ string, mod func(*msg.Event)) {
	e := msg.Event{SchemaVersion: msg.SchemaVersion, Station: a.Station, BookingID: p.BookingID, Type: typ, T: clk.Now(), Ts: time.Now().UTC()}
	if mod != nil {
		mod(&e)
	}
	a.publish("events", e, false)
	slog.Info("event", "type", typ, "booking", p.BookingID)
}

func (a *Agent) onCommand(_ string, payload []byte) {
	var p msg.Plan
	if err := json.Unmarshal(payload, &p); err != nil || p.Type != "plan" {
		slog.Warn("ignoring command", "err", err)
		return
	}
	clk := newSimClock(p.SimNow, p.SimScale)
	select {
	case a.Busy <- struct{}{}:
		go func() {
			defer func() { <-a.Busy }()
			a.runPass(&p, clk)
		}()
	default:
		a.event(&p, clk, msg.EvError, func(e *msg.Event) { e.Detail = "station busy" })
	}
}

// runPass: ack, pre-position, wait for AOS, track at 1 Hz streaming telemetry, then upload at LOS.
func (a *Agent) runPass(p *msg.Plan, clk simClock) {
	slog.Info("pass plan received", "booking", p.BookingID, "satellite", p.Satellite, "aos", p.AOS, "los", p.LOS, "scale", p.SimScale)
	a.event(p, clk, msg.EvPlanAck, nil)
	if len(p.Track) == 0 {
		a.event(p, clk, msg.EvError, func(e *msg.Event) { e.Detail = "empty track" })
		return
	}
	// 1. pre-position and 2. tune
	if err := a.Rot.Point(p.Track[0].Az, p.Track[0].El); err != nil {
		a.event(p, clk, msg.EvError, func(e *msg.Event) { e.Detail = "rotator: " + err.Error() })
		return
	}
	if err := a.Mod.Tune(p.FreqHz, p.Mode); err != nil {
		a.event(p, clk, msg.EvError, func(e *msg.Event) { e.Detail = "modem: " + err.Error() })
		return
	}
	a.event(p, clk, msg.EvPrepos, nil)
	clk.SleepUntil(p.AOS)

	// 3. AOS: track
	a.setState(msg.StateTrack)
	a.event(p, clk, msg.EvAOS, nil)
	_ = a.Mod.Record(true)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for ; ; <-tick.C {
		now := clk.Now()
		if !now.Before(p.LOS) {
			break
		}
		az, el := interpolate(p.Track, now)
		if err := a.Rot.Point(az, el); err != nil {
			slog.Error("rotator point", "err", err)
		}
		paz, pel, err := a.Rot.Position()
		if err != nil {
			slog.Error("rotator position", "err", err)
		}
		st, err := a.Mod.Status()
		if err != nil {
			slog.Error("modem status", "err", err)
		}
		// 4. telemetry frame
		a.publish("telemetry", msg.Telemetry{SchemaVersion: msg.SchemaVersion, Station: a.Station, BookingID: p.BookingID, T: now,
			Az: paz, El: pel, SNR: st.SNR, Lock: st.Lock, Bitrate: st.Bitrate}, false)
	}

	// 5. LOS: stop recording, upload mission data
	losAt := time.Now()
	_ = a.Mod.Record(false)
	data, err := a.Mod.Dump()
	a.setState(msg.StateIdle)
	a.event(p, clk, msg.EvLOS, nil)
	if err != nil {
		a.event(p, clk, msg.EvError, func(e *msg.Event) { e.Detail = "dump: " + err.Error() })
		return
	}
	if err := upload(p, data); err != nil {
		a.event(p, clk, msg.EvError, func(e *msg.Event) { e.Detail = "upload: " + err.Error() })
		return
	}
	a.event(p, clk, msg.EvUploadOK, func(e *msg.Event) { e.Bytes, e.DelayMs = int64(len(data)), time.Since(losAt).Milliseconds() })
}

// upload requests a presigned URL (authorised by the plan token) and PUTs the mission file.
func upload(p *msg.Plan, data []byte) error {
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/bookings/%s/upload-url", p.APIBase, p.BookingID), nil)
	req.Header.Set("X-Upload-Token", p.UploadToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var u struct{ Url, Key string }
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&u) != nil {
		return fmt.Errorf("upload-url: status %d", resp.StatusCode)
	}
	put, _ := http.NewRequest(http.MethodPut, u.Url, bytes.NewReader(data))
	put.ContentLength = int64(len(data))
	r2, err := http.DefaultClient.Do(put)
	if err != nil {
		return err
	}
	defer r2.Body.Close()
	if r2.StatusCode/100 != 2 {
		return fmt.Errorf("PUT %s: status %d", u.Key, r2.StatusCode)
	}
	slog.Info("mission data uploaded", "key", u.Key, "bytes", len(data))
	return nil
}
