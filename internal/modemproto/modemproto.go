// Package modemproto implements a vendor-style line protocol for a demodulator control port over TCP:
//
//	FREQ <hz>        -> OK
//	MODE <name>      -> OK
//	REC START|STOP   -> OK
//	LOCK?            -> LOCK <0|1> SNR <db> BITRATE <bps>
//	DUMP             -> DATA <n>\n<n raw bytes>
package modemproto

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Status is the reply to LOCK?.
type Status struct {
	Lock    bool
	SNR     float64
	Bitrate int
}

// FormatStatus renders a LOCK? reply.
func FormatStatus(s Status) string {
	l := 0
	if s.Lock {
		l = 1
	}
	return fmt.Sprintf("LOCK %d SNR %.1f BITRATE %d", l, s.SNR, s.Bitrate)
}

// ParseStatus parses a LOCK? reply.
func ParseStatus(line string) (Status, error) {
	f := strings.Fields(line)
	if len(f) != 6 || f[0] != "LOCK" || f[2] != "SNR" || f[4] != "BITRATE" {
		return Status{}, fmt.Errorf("bad status line %q", line)
	}
	snr, err := strconv.ParseFloat(f[3], 64)
	if err != nil {
		return Status{}, err
	}
	br, err := strconv.Atoi(f[5])
	if err != nil {
		return Status{}, err
	}
	return Status{Lock: f[1] == "1", SNR: snr, Bitrate: br}, nil
}

// Client is the agent-side driver of the modem control port.
type Client struct {
	mu sync.Mutex
	c  net.Conn
	r  *bufio.Reader
}

// Dial connects to the control port.
func Dial(addr string) (*Client, error) {
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	return &Client{c: c, r: bufio.NewReader(c)}, nil
}

func (m *Client) line(cmd string) (string, error) {
	slog.Info("modem tx", "bytes", cmd)
	if _, err := fmt.Fprintf(m.c, "%s\n", cmd); err != nil {
		return "", err
	}
	l, err := m.r.ReadString('\n')
	l = strings.TrimSpace(l)
	slog.Info("modem rx", "bytes", l)
	return l, err
}

func (m *Client) expectOK(cmd string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, err := m.line(cmd)
	if err != nil {
		return err
	}
	if l != "OK" {
		return fmt.Errorf("modem %q: %s", cmd, l)
	}
	return nil
}

// Tune sets the receive frequency and demodulator mode.
func (m *Client) Tune(freqHz int64, mode string) error {
	if err := m.expectOK(fmt.Sprintf("FREQ %d", freqHz)); err != nil {
		return err
	}
	return m.expectOK("MODE " + mode)
}

// Record starts or stops mission-data recording.
func (m *Client) Record(on bool) error {
	if on {
		return m.expectOK("REC START")
	}
	return m.expectOK("REC STOP")
}

// Status polls LOCK?.
func (m *Client) Status() (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, err := m.line("LOCK?")
	if err != nil {
		return Status{}, err
	}
	return ParseStatus(l)
}

// Dump fetches the recorded data.
func (m *Client) Dump() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, err := m.line("DUMP")
	if err != nil {
		return nil, err
	}
	var n int
	if _, err := fmt.Sscanf(l, "DATA %d", &n); err != nil {
		return nil, fmt.Errorf("bad DUMP reply %q", l)
	}
	buf := make([]byte, n)
	_, err = io.ReadFull(m.r, buf)
	return buf, err
}

// Close closes the connection.
func (m *Client) Close() error { return m.c.Close() }
