package notifier

import (
	"context"
	"log/slog"
	"time"
)

type Payload struct {
	CourierID string
	Topic     string
	Lat       float64
	Lng       float64
	Recorded  time.Time
}

type Notifier interface {
	Send(ctx context.Context, payload Payload) error
}

type MockNotifier struct {
	logger *slog.Logger
}

func NewMock(logger *slog.Logger) *MockNotifier {
	return &MockNotifier{logger: logger}
}

func (m *MockNotifier) Send(ctx context.Context, payload Payload) error {
	m.logger.Info("mock notification", "courier_id", payload.CourierID, "topic", payload.Topic, "lat", payload.Lat, "lng", payload.Lng, "recorded_at", payload.Recorded)
	return nil
}
