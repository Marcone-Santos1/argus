package usage

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	js := `{"session_id":"abc","context_window":{"used_percentage":42.5},
"rate_limits":{"five_hour":{"used_percentage":34,"resets_at":1795300000},"seven_day":{"used_percentage":12.4,"resets_at":"2026-11-25T10:00:00Z"}}}`
	in, ok := Parse([]byte(js), time.Now())
	if !ok || in.Session != "abc" || in.Context != 42.5 {
		t.Fatalf("parse: %+v", in)
	}
	if in.FiveHour == nil || in.FiveHour.Percent != 34 || in.FiveHour.Resets.Unix() != 1795300000 {
		t.Fatalf("5h: %+v", in.FiveHour)
	}
	if in.SevenDay == nil || in.SevenDay.Resets.Year() != 2026 {
		t.Fatalf("7d: %+v", in.SevenDay)
	}
	in, ok = Parse([]byte(`{"session_id":"x"}`), time.Now())
	if !ok || in.FiveHour != nil || in.Context != -1 {
		t.Fatalf("without limits (API key, or before the first reply): %+v", in)
	}
	if _, ok := Parse([]byte(`not json`), time.Now()); ok {
		t.Fatal("bad JSON")
	}
	if !(&Limit{Percent: 1, Resets: time.Now().Add(-time.Minute)}).Expired(time.Now()) {
		t.Fatal("a window that reset is expired")
	}
}
