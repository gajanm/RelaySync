package cache

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestRedisLatestLocation(t *testing.T) {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL not set")
	}
	client, err := NewClient(url, "test-stream")
	if err != nil {
		t.Fatalf("failed to create redis client: %v", err)
	}
	ctx := context.Background()
	loc := Location{
		CourierID:  "test-courier",
		Lat:        10.0,
		Lng:        20.0,
		AccuracyM:  5,
		RecordedAt: time.Now().UTC(),
		Source:     "test",
		EventID:    "evt-1",
	}
	if err := client.WriteLatest(ctx, loc, 5*time.Second); err != nil {
		t.Fatalf("write latest failed: %v", err)
	}
	got, err := client.GetLatest(ctx, loc.CourierID)
	if err != nil {
		t.Fatalf("get latest failed: %v", err)
	}
	if got.CourierID != loc.CourierID {
		t.Fatalf("unexpected courier id")
	}
}
