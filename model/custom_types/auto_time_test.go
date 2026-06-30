package custom_types

import (
	"testing"
	"time"
)

func TestAutoTimeMarshalJSONUsesRFC3339WithTimezone(t *testing.T) {
	ts := time.Date(2026, 6, 30, 5, 50, 33, 0, time.UTC)
	got, err := AutoTime(ts).MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON returned error: %v", err)
	}
	if string(got) != `"2026-06-30T05:50:33Z"` {
		t.Fatalf("expected RFC3339 UTC timestamp, got %s", got)
	}
}

func TestAutoTimeMarshalJSONPreservesOffset(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	ts := time.Date(2026, 6, 30, 13, 50, 33, 0, loc)
	got, err := AutoTime(ts).MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON returned error: %v", err)
	}
	if string(got) != `"2026-06-30T13:50:33+08:00"` {
		t.Fatalf("expected timezone offset to be preserved, got %s", got)
	}
}

func TestAutoTimeMarshalJSONZeroReturnsNull(t *testing.T) {
	got, err := AutoTime(time.Time{}).MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON returned error: %v", err)
	}
	if string(got) != `null` {
		t.Fatalf("expected zero AutoTime to marshal as null, got %s", got)
	}
}
