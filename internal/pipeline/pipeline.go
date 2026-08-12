package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"relaysync/internal/metrics"
	"relaysync/internal/notifier"
	"relaysync/internal/store/postgres"
)

type Consumer struct {
	logger         *slog.Logger
	redis          *redis.Client
	store          *postgres.Store
	stream         string
	group          string
	consumer       string
	batchSize      int
	batchWindow    time.Duration
	block          time.Duration
	notifications  chan notifier.Payload
	notifier       notifier.Notifier
	notifierWorker int
	topicPrefix    string
}

func NewConsumer(logger *slog.Logger, redisClient *redis.Client, store *postgres.Store, notifier notifier.Notifier, stream, group, consumer string, batchSize int, batchWindow, block time.Duration, notifQueue int, notifWorkers int, topicPrefix string) *Consumer {
	return &Consumer{
		logger:         logger,
		redis:          redisClient,
		store:          store,
		stream:         stream,
		group:          group,
		consumer:       consumer,
		batchSize:      batchSize,
		batchWindow:    batchWindow,
		block:          block,
		notifications:  make(chan notifier.Payload, notifQueue),
		notifier:       notifier,
		notifierWorker: notifWorkers,
		topicPrefix:    topicPrefix,
	}
}

func (c *Consumer) Start(ctx context.Context) error {
	if err := c.ensureGroup(ctx); err != nil {
		return err
	}
	for i := 0; i < c.notifierWorker; i++ {
		go c.notifierLoop(ctx)
	}
	go c.consumeLoop(ctx)
	return nil
}

func (c *Consumer) ensureGroup(ctx context.Context) error {
	err := c.redis.XGroupCreateMkStream(ctx, c.stream, c.group, "0").Err()
	if err != nil && !errors.Is(err, redis.BusyGroupError{}) {
		if err.Error() != "BUSYGROUP Consumer Group name already exists" {
			return err
		}
	}
	return nil
}

func (c *Consumer) consumeLoop(ctx context.Context) {
	ticker := time.NewTicker(c.batchWindow)
	defer ticker.Stop()

	var batch []redis.XMessage
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if len(batch) > 0 {
				c.flush(ctx, batch)
				batch = nil
			}
		default:
			streams, err := c.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group:    c.group,
				Consumer: c.consumer,
				Streams:  []string{c.stream, ">"},
				Count:    int64(c.batchSize),
				Block:    c.block,
			}).Result()
			if err != nil && !errors.Is(err, redis.Nil) {
				c.logger.Error("stream read failed", "error", err)
				continue
			}
			if lag, err := c.redis.XLen(ctx, c.stream).Result(); err == nil {
				metrics.StreamLag.Set(float64(lag))
			}
			for _, stream := range streams {
				batch = append(batch, stream.Messages...)
			}
			if len(batch) >= c.batchSize {
				c.flush(ctx, batch)
				batch = nil
			}
		}
	}
}

func (c *Consumer) flush(ctx context.Context, messages []redis.XMessage) {
	events := make([]postgres.LocationEvent, 0, len(messages))
	ackIDs := make([]string, 0, len(messages))
	for _, msg := range messages {
		parsed, err := parseMessage(msg)
		if err != nil {
			c.logger.Error("failed to parse message", "error", err)
			ackIDs = append(ackIDs, msg.ID)
			continue
		}
		events = append(events, parsed)
		ackIDs = append(ackIDs, msg.ID)
		c.enqueueNotification(parsed)
	}

	metrics.BatchInsertSize.Observe(float64(len(events)))
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := c.store.InsertLocationEvents(dbCtx, events); err != nil {
		c.logger.Error("batch insert failed", "error", err)
		return
	}
	if len(ackIDs) > 0 {
		ackCtx, ackCancel := context.WithTimeout(ctx, 2*time.Second)
		defer ackCancel()
		if err := c.redis.XAck(ackCtx, c.stream, c.group, ackIDs...).Err(); err != nil {
			c.logger.Error("failed to ack stream", "error", err)
		}
	}
}

func parseMessage(msg redis.XMessage) (postgres.LocationEvent, error) {
	getString := func(key string) (string, error) {
		val, ok := msg.Values[key]
		if !ok {
			return "", errors.New("missing field: " + key)
		}
		switch t := val.(type) {
		case string:
			return t, nil
		case []byte:
			return string(t), nil
		default:
			return "", errors.New("invalid field type")
		}
	}

	courierID, err := getString("courier_id")
	if err != nil {
		return postgres.LocationEvent{}, err
	}
	latStr, err := getString("lat")
	if err != nil {
		return postgres.LocationEvent{}, err
	}
	lngStr, err := getString("lng")
	if err != nil {
		return postgres.LocationEvent{}, err
	}
	accuracyStr, err := getString("accuracy_m")
	if err != nil {
		return postgres.LocationEvent{}, err
	}
	recordedStr, err := getString("recorded_at")
	if err != nil {
		return postgres.LocationEvent{}, err
	}
	source, _ := getString("source")
	eventID, _ := getString("event_id")

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		return postgres.LocationEvent{}, err
	}
	lng, err := strconv.ParseFloat(lngStr, 64)
	if err != nil {
		return postgres.LocationEvent{}, err
	}
	accuracy, err := strconv.ParseFloat(accuracyStr, 64)
	if err != nil {
		return postgres.LocationEvent{}, err
	}
	recordedAt, err := time.Parse(time.RFC3339Nano, recordedStr)
	if err != nil {
		return postgres.LocationEvent{}, err
	}
	metadata := map[string]string{}
	if raw, ok := msg.Values["metadata"]; ok {
		switch t := raw.(type) {
		case string:
			_ = json.Unmarshal([]byte(t), &metadata)
		case []byte:
			_ = json.Unmarshal(t, &metadata)
		}
	}

	return postgres.LocationEvent{
		EventID:    eventID,
		CourierID:  courierID,
		Lat:        lat,
		Lng:        lng,
		AccuracyM:  accuracy,
		RecordedAt: recordedAt,
		Source:     source,
		Metadata:   metadata,
	}, nil
}

func (c *Consumer) enqueueNotification(event postgres.LocationEvent) {
	payload := notifier.Payload{
		CourierID: event.CourierID,
		Topic:     c.topicPrefix + event.CourierID,
		Lat:       event.Lat,
		Lng:       event.Lng,
		Recorded:  event.RecordedAt,
	}
	select {
	case c.notifications <- payload:
	default:
		c.logger.Warn("notification queue full", "courier_id", event.CourierID)
	}
}

func (c *Consumer) notifierLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-c.notifications:
			start := time.Now()
			sendCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			if err := c.notifier.Send(sendCtx, payload); err != nil {
				metrics.NotificationsSent.WithLabelValues("error").Inc()
				c.logger.Error("notification failed", "error", err)
			} else {
				metrics.NotificationsSent.WithLabelValues("ok").Inc()
			}
			cancel()
			metrics.NotifierLatency.Observe(time.Since(start).Seconds())
		}
	}
}
