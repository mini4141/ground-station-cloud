// Package msg defines the JSON messages exchanged over MQTT between cloud and edge.
// Topics (QoS 1): gs/{station}/cmd, telemetry, status (retained), events.
package msg

import (
	"fmt"
	"time"
)

// SchemaVersion is carried by every message.
const SchemaVersion = 1

// Topic builds gs/{station}/{kind}.
func Topic(station, kind string) string { return fmt.Sprintf("gs/%s/%s", station, kind) }

// TrackPoint is one az/el sample of the predicted track (simulated time).
type TrackPoint struct {
	T  time.Time `json:"t"`
	Az float64   `json:"az"`
	El float64   `json:"el"`
}

// Plan is the cloud-to-edge pass plan published to gs/{station}/cmd.
type Plan struct {
	SchemaVersion int          `json:"schemaVersion"`
	Type          string       `json:"type"` // "plan"
	BookingID     string       `json:"bookingId"`
	Station       string       `json:"station"`
	Satellite     string       `json:"satellite"`
	AOS           time.Time    `json:"aos"`
	LOS           time.Time    `json:"los"`
	FreqHz        int64        `json:"freqHz"`
	Mode          string       `json:"mode"`
	Track         []TrackPoint `json:"track"`
	SimNow        time.Time    `json:"simNow"`   // simulated time when the plan was sent
	SimScale      float64      `json:"simScale"` // simulated seconds per real second
	APIBase       string       `json:"apiBase"`
	UploadToken   string       `json:"uploadToken"`
	SentAt        time.Time    `json:"sentAt"`
}

// Telemetry is a 1 Hz frame published to gs/{station}/telemetry.
type Telemetry struct {
	SchemaVersion int       `json:"schemaVersion"`
	Station       string    `json:"station"`
	BookingID     string    `json:"bookingId"`
	T             time.Time `json:"t"` // simulated time
	Az            float64   `json:"az"`
	El            float64   `json:"el"`
	SNR           float64   `json:"snr"`
	Lock          bool      `json:"lock"`
	Bitrate       int       `json:"bitrate"`
}

// Event types published to gs/{station}/events.
const (
	EvPlanAck    = "plan_ack"
	EvPrepos     = "prepositioned"
	EvAOS        = "AOS"
	EvLOS        = "LOS"
	EvUploadOK   = "upload_ok"
	EvError      = "error"
	StateIdle    = "idle"
	StateTrack   = "tracking"
	StateOffline = "offline"
)

// Event is published to gs/{station}/events.
type Event struct {
	SchemaVersion int       `json:"schemaVersion"`
	Station       string    `json:"station"`
	BookingID     string    `json:"bookingId"`
	Type          string    `json:"type"`
	T             time.Time `json:"t"`  // simulated time
	Ts            time.Time `json:"ts"` // wall clock
	Bytes         int64     `json:"bytes,omitempty"`
	DelayMs       int64     `json:"delayMs,omitempty"`
	Detail        string    `json:"detail,omitempty"`
}

// Status is the retained heartbeat on gs/{station}/status.
type Status struct {
	SchemaVersion int       `json:"schemaVersion"`
	Station       string    `json:"station"`
	State         string    `json:"state"`
	Ts            time.Time `json:"ts"`
}

// Envelope is what lands on the SQS bus: the original message plus station/kind
// (added by the IoT topic rule in AWS and by the bridge locally).
type Envelope struct {
	Kind    string `json:"kind"`
	Station string `json:"station"`
}
