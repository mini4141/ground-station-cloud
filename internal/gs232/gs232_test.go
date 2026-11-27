package gs232

import "testing"

func TestRoundTrip(t *testing.T) {
	if got := FormatSet(123.4, 45.6); got != "W123 046" {
		t.Fatalf("FormatSet = %q", got)
	}
	az, el, err := ParseSet("W123 046")
	if err != nil || az != 123 || el != 46 {
		t.Fatalf("ParseSet = %d %d %v", az, el, err)
	}
	az, el, err = ParsePosition(FormatPosition(270, 12))
	if err != nil || az != 270 || el != 12 {
		t.Fatalf("ParsePosition = %d %d %v", az, el, err)
	}
	if _, _, err := ParsePosition("garbage"); err == nil {
		t.Fatal("expected error")
	}
}
