package pipeline

import (
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestParseMessage(t *testing.T) {
	msg := redis.XMessage{
		ID: "1-1",
		Values: map[string]interface{}{
			"courier_id":  "courier-1",
			"lat":         "37.0",
			"lng":         "-122.0",
			"accuracy_m":  "5",
			"recorded_at": time.Now().UTC().Format(time.RFC3339Nano),
			"source":      "ingest",
			"event_id":    "evt-1",
		},
	}

	event, err := parseMessage(msg)
	if err != nil {
		t.Fatalf("expected parse ok: %v", err)
	}
	if event.CourierID != "courier-1" {
		t.Fatalf("unexpected courier id: %s", event.CourierID)
	}
}
