// Package gs232 implements the Yaesu GS-232 antenna rotator command subset over any byte stream
// (a serial port in the container, or TCP as fallback): W (point), C2 (read position), S (stop).
package gs232

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
)

// FormatSet builds the "Waaa eee" command.
func FormatSet(az, el float64) string {
	return fmt.Sprintf("W%03d %03d", clamp(az, 0, 450), clamp(el, 0, 180))
}

func clamp(v float64, lo, hi int) int {
	return int(math.Max(float64(lo), math.Min(float64(hi), math.Round(v))))
}

// ParseSet parses "Waaa eee".
func ParseSet(line string) (az, el int, err error) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "W") {
		return 0, 0, fmt.Errorf("not a W command: %q", line)
	}
	f := strings.Fields(line[1:])
	if len(f) != 2 {
		return 0, 0, fmt.Errorf("W needs two arguments: %q", line)
	}
	if az, err = strconv.Atoi(f[0]); err != nil {
		return
	}
	el, err = strconv.Atoi(f[1])
	return
}

// FormatPosition builds the C2 reply "AZ=aaa  EL=eee".
func FormatPosition(az, el int) string { return fmt.Sprintf("AZ=%03d  EL=%03d", az, el) }

// ParsePosition parses the C2 reply.
func ParsePosition(line string) (az, el int, err error) {
	var a, e string
	for _, f := range strings.Fields(line) {
		switch {
		case strings.HasPrefix(f, "AZ="):
			a = f[3:]
		case strings.HasPrefix(f, "EL="):
			e = f[3:]
		}
	}
	if a == "" || e == "" {
		return 0, 0, fmt.Errorf("bad position reply %q", line)
	}
	if az, err = strconv.Atoi(a); err != nil {
		return
	}
	el, err = strconv.Atoi(e)
	return
}

// Client talks GS-232 over a stream.
type Client struct {
	mu sync.Mutex
	rw io.ReadWriteCloser
	r  *bufio.Reader
}

// NewClient wraps a stream.
func NewClient(rw io.ReadWriteCloser) *Client { return &Client{rw: rw, r: bufio.NewReader(rw)} }

func (c *Client) send(cmd string) error {
	slog.Info("rotator tx", "bytes", cmd)
	_, err := c.rw.Write([]byte(cmd + "\r"))
	return err
}

// Point commands the rotator (GS-232 does not reply to W).
func (c *Client) Point(az, el float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.send(FormatSet(az, el))
}

// Position reads the current az/el with C2.
func (c *Client) Position() (az, el float64, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err = c.send("C2"); err != nil {
		return
	}
	line, err := c.r.ReadString('\r')
	if err != nil {
		return 0, 0, err
	}
	line = strings.TrimSpace(line)
	slog.Info("rotator rx", "bytes", line)
	a, e, err := ParsePosition(line)
	return float64(a), float64(e), err
}

// Close closes the stream.
func (c *Client) Close() error { return c.rw.Close() }
