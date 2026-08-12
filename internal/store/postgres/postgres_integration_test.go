package postgres

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestStoreInsertLocationEvents(t *testing.T) {
	url := os.Getenv("POSTGRES_URL")
	if url == "" {
		t.Skip("POSTGRES_URL not set")
	}
	ctx := context.Background()
	store, err := New(ctx, url)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer store.Close()

	courierID := "test-courier-store"
	_, _ = store.CreateCourier(ctx, Courier{ID: courierID, Name: "Test Courier", Status: "active"})

	events := []LocationEvent{
		{
			EventID:    "evt-store-1",
			CourierID:  courierID,
			Lat:        1.0,
			Lng:        2.0,
			AccuracyM:  3,
			RecordedAt: time.Now().UTC(),
			Source:     "test",
			Metadata:   map[string]string{"source": "test"},
		},
	}
	if err := store.InsertLocationEvents(ctx, events); err != nil {
		t.Fatalf("insert events failed: %v", err)
	}
}
