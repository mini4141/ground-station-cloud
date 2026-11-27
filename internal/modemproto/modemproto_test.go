package modemproto

import "testing"

func TestStatusRoundTrip(t *testing.T) {
	in := Status{Lock: true, SNR: 12.3, Bitrate: 9600}
	got, err := ParseStatus(FormatStatus(in))
	if err != nil || got != in {
		t.Fatalf("got %+v err %v", got, err)
	}
	if _, err := ParseStatus("LOCK maybe"); err == nil {
		t.Fatal("expected error")
	}
}
