// Command sim-modem simulates a demodulator control port (TCP line protocol).
// Signal quality follows the real antenna elevation published by the rotator sim.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mini4141/ground-station-cloud/internal/modemproto"
	"github.com/mini4141/ground-station-cloud/internal/platform"
)

const bytesPerLockedSecond = 1200

type modem struct {
	mu        sync.Mutex
	world     string
	freq      int64
	mode      string
	recording bool
	last      time.Time
	recorded  float64
}

func (m *modem) antennaEl() float64 {
	b, err := os.ReadFile(m.world)
	if err != nil {
		return 0
	}
	var w struct{ El float64 }
	_ = json.Unmarshal(b, &w)
	return w.El
}

func (m *modem) status() modemproto.Status {
	el := m.antennaEl()
	tuned := m.freq > 0 && m.mode != ""
	s := modemproto.Status{SNR: 0.5}
	if tuned && el >= 3 {
		s.Lock, s.Bitrate = true, 9600
		s.SNR = 3 + 16*math.Sin(el*math.Pi/180) + math.Sin(float64(time.Now().UnixMilli())/700)
	}
	now := time.Now()
	if m.recording && s.Lock && !m.last.IsZero() {
		m.recorded += now.Sub(m.last).Seconds() * bytesPerLockedSecond
	}
	m.last = now
	return s
}

func (m *modem) handle(c net.Conn) {
	defer c.Close()
	sc := bufio.NewScanner(c)
	for sc.Scan() {
		cmd := strings.TrimSpace(sc.Text())
		slog.Info("modem sim rx", "bytes", cmd)
		m.mu.Lock()
		switch {
		case strings.HasPrefix(cmd, "FREQ "):
			_, _ = fmt.Sscanf(cmd, "FREQ %d", &m.freq)
			fmt.Fprint(c, "OK\n")
		case strings.HasPrefix(cmd, "MODE "):
			m.mode = strings.TrimPrefix(cmd, "MODE ")
			fmt.Fprint(c, "OK\n")
		case cmd == "REC START":
			m.recording, m.recorded, m.last = true, 0, time.Now()
			fmt.Fprint(c, "OK\n")
		case cmd == "REC STOP":
			m.status()
			m.recording = false
			fmt.Fprint(c, "OK\n")
		case cmd == "LOCK?":
			fmt.Fprintf(c, "%s\n", modemproto.FormatStatus(m.status()))
		case cmd == "DUMP":
			n := int(m.recorded)
			fmt.Fprintf(c, "DATA %d\n", n)
			buf := make([]byte, n)
			for i := range buf {
				buf[i] = byte(i*31 + 7)
			}
			_, _ = c.Write(buf)
		default:
			fmt.Fprint(c, "ERR unknown command\n")
		}
		m.mu.Unlock()
	}
}

func main() {
	platform.Setup(context.Background(), "sim-modem")
	m := &modem{world: platform.Env("SIM_WORLD", "/tmp/antenna.json")}
	addr := platform.Env("MODEM_SIM_LISTEN", ":7777")
	l, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
	slog.Info("modem sim listening", "addr", addr)
	for {
		c, err := l.Accept()
		if err != nil {
			return
		}
		go m.handle(c)
	}
}
