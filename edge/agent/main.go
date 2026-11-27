// Command agent is the ground station edge agent: it receives pass plans over MQTT (mTLS),
// drives the rotator (GS-232 over serial) and modem (TCP), streams telemetry and uploads mission data.
package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"time"

	"go.bug.st/serial"

	"github.com/mini4141/ground-station-cloud/internal/gs232"
	"github.com/mini4141/ground-station-cloud/internal/modemproto"
	"github.com/mini4141/ground-station-cloud/internal/mqtt"
	"github.com/mini4141/ground-station-cloud/internal/msg"
	"github.com/mini4141/ground-station-cloud/internal/platform"
)

// Rotator is the antenna driver interface; the agent does not know what is behind it.
type Rotator interface {
	Point(az, el float64) error
	Position() (az, el float64, err error)
}

// Modem is the receiver driver interface.
type Modem interface {
	Tune(freqHz int64, mode string) error
	Record(on bool) error
	Status() (modemproto.Status, error)
	Dump() ([]byte, error)
}

func openRotator() Rotator {
	for {
		var rw io.ReadWriteCloser
		var err error
		if dev := os.Getenv("ROTATOR_DEV"); dev != "" {
			rw, err = serial.Open(dev, &serial.Mode{BaudRate: 9600})
		} else { // TCP fallback for environments without pty support
			rw, err = net.DialTimeout("tcp", platform.Env("ROTATOR_TCP", "localhost:4533"), 3*time.Second)
		}
		if err == nil {
			return gs232.NewClient(rw)
		}
		slog.Warn("rotator not ready", "err", err)
		time.Sleep(time.Second)
	}
}

func openModem() Modem {
	for {
		m, err := modemproto.Dial(platform.Env("MODEM_ADDR", "localhost:7777"))
		if err == nil {
			return m
		}
		slog.Warn("modem not ready", "err", err)
		time.Sleep(time.Second)
	}
}

func main() {
	ctx := context.Background()
	defer platform.Setup(ctx, "agent")()
	station := platform.Env("STATION", "tokyo")
	slog.SetDefault(slog.Default().With("station", station))

	a := &Agent{Station: station, Rot: openRotator(), Mod: openModem(), Busy: make(chan struct{}, 1)}
	offline, _ := json.Marshal(msg.Status{SchemaVersion: msg.SchemaVersion, Station: station, State: msg.StateOffline, Ts: time.Now().UTC()})
	c, err := mqtt.Connect(mqtt.Options{URL: platform.Env("MQTT_URL", "ssl://mosquitto:8883"), ClientID: "agent-" + station,
		CA: platform.Env("MQTT_CA", "/certs/ca.crt"), Cert: platform.Env("MQTT_CERT", "/certs/"+station+".crt"), Key: platform.Env("MQTT_KEY", "/certs/"+station+".key"),
		WillTopic: msg.Topic(station, "status"), WillPayload: offline,
		OnConnect: func(c *mqtt.Client) {
			if err := c.Subscribe(msg.Topic(station, "cmd"), a.onCommand); err != nil {
				slog.Error("subscribe cmd", "err", err)
			}
		}})
	if err != nil {
		slog.Error("mqtt connect", "err", err)
		os.Exit(1)
	}
	a.MQ = c
	a.heartbeat(ctx)
}
