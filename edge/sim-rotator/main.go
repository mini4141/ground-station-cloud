// Command sim-rotator simulates a GS-232 antenna rotator on a serial port (socat pty) or TCP.
// It slews at a finite rate and publishes the real antenna pointing to a "world" file the modem sim reads.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"

	"github.com/mini4141/ground-station-cloud/internal/gs232"
	"github.com/mini4141/ground-station-cloud/internal/platform"
)

type rotator struct {
	mu                         sync.Mutex
	az, el, targetAz, targetEl float64
}

func (r *rotator) run(rate float64, world string) {
	const dt = 100 * time.Millisecond
	for range time.Tick(dt) {
		r.mu.Lock()
		step := rate * dt.Seconds()
		r.az += clampStep(r.targetAz-r.az, step)
		r.el += clampStep(r.targetEl-r.el, step)
		b, _ := json.Marshal(map[string]float64{"az": r.az, "el": r.el})
		r.mu.Unlock()
		_ = os.WriteFile(world, b, 0o644)
	}
}

func clampStep(d, step float64) float64 { return math.Max(-step, math.Min(step, d)) }

func (r *rotator) serve(rw io.ReadWriter) {
	br := bufio.NewReader(rw)
	for {
		line, err := br.ReadString('\r')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		slog.Info("rotator sim rx", "bytes", line)
		switch {
		case strings.HasPrefix(line, "W"):
			if az, el, err := gs232.ParseSet(line); err == nil {
				r.mu.Lock()
				r.targetAz, r.targetEl = float64(az), float64(el)
				r.mu.Unlock()
			}
		case line == "C2":
			r.mu.Lock()
			reply := gs232.FormatPosition(int(math.Round(r.az)), int(math.Round(r.el)))
			r.mu.Unlock()
			_, _ = rw.Write([]byte(reply + "\r"))
		case line == "S":
			r.mu.Lock()
			r.targetAz, r.targetEl = r.az, r.el
			r.mu.Unlock()
		}
	}
}

func main() {
	platform.Setup(context.Background(), "sim-rotator")
	r := &rotator{}
	go r.run(platform.EnvFloat("SLEW_DEG_PER_S", 60), platform.Env("SIM_WORLD", "/tmp/antenna.json"))
	if dev := os.Getenv("ROTATOR_SIM_DEV"); dev != "" {
		p, err := serial.Open(dev, &serial.Mode{BaudRate: 9600})
		if err != nil {
			slog.Error("open serial", "dev", dev, "err", err)
			os.Exit(1)
		}
		slog.Info("rotator sim on serial", "dev", dev)
		r.serve(p)
		return
	}
	addr := platform.Env("ROTATOR_SIM_LISTEN", ":4533")
	l, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
	slog.Info("rotator sim on tcp", "addr", addr)
	for {
		c, err := l.Accept()
		if err != nil {
			return
		}
		go r.serve(c)
	}
}
