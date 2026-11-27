// Command seed loads demo satellites from the bundled TLE snapshots and sets the simulated clock.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mini4141/ground-station-cloud/internal/api"
)

func main() {
	endpoint := flag.String("api", "http://localhost:8080", "API base URL")
	token := flag.String("token", "", "bearer token")
	dir := flag.String("tle-dir", "data/tle", "TLE snapshot directory")
	clock := flag.String("clock", "2026-11-27T09:30:00Z", "simulated start time")
	flag.Parse()

	ctx := context.Background()
	c, err := api.NewClientWithResponses(*endpoint, api.WithRequestEditorFn(func(_ context.Context, r *http.Request) error {
		r.Header.Set("Authorization", "Bearer "+*token)
		return nil
	}))
	if err != nil {
		log.Fatal(err)
	}
	existing, err := c.ListSatellitesWithResponse(ctx)
	if err != nil || existing.JSON200 == nil {
		log.Fatalf("list satellites: %v", err)
	}
	have := map[string]bool{}
	for _, s := range *existing.JSON200 {
		have[s.Name] = true
	}
	files, _ := filepath.Glob(filepath.Join(*dir, "*.tle"))
	for _, f := range files {
		b, _ := os.ReadFile(f)
		lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(string(b)), "\r", ""), "\n")
		if len(lines) < 3 {
			continue
		}
		name, l1, l2 := strings.TrimSpace(lines[0]), strings.TrimRight(lines[1], " "), strings.TrimRight(lines[2], " ")
		if have[name] {
			continue
		}
		var norad int32
		for _, f := range strings.Fields(l1)[1:2] {
			for _, ch := range strings.TrimRight(f, "U") {
				norad = norad*10 + int32(ch-'0')
			}
		}
		out, err := c.CreateSatelliteWithResponse(ctx, api.SatelliteInput{Name: name, NoradId: norad, Tle1: l1, Tle2: l2})
		if err != nil || out.JSON201 == nil {
			log.Fatalf("create satellite %s: %v status %d", name, err, out.StatusCode())
		}
		log.Printf("satellite %s -> %s", name, out.JSON201.Id)
	}
	t, _ := time.Parse(time.RFC3339, *clock)
	if _, err := c.SetSimClockWithResponse(ctx, api.SetSimClockJSONRequestBody{SimTime: t}); err != nil {
		log.Fatal(err)
	}
	log.Printf("sim clock set to %s", t)
}
