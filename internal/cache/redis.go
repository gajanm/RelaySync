package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	client *redis.Client
	stream string
}

type Location struct {
	CourierID  string            `json:"courier_id"`
	Lat        float64           `json:"lat"`
	Lng        float64           `json:"lng"`
	AccuracyM  float64           `json:"accuracy_m"`
	RecordedAt time.Time         `json:"recorded_at"`
	Source     string            `json:"source"`
	Metadata   map[string]string `json:"metadata"`
	EventID    string            `json:"event_id"`
}

func NewClient(url, stream string) (*Client, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	return &Client{client: redis.NewClient(opt), stream: stream}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *Client) WriteLatest(ctx context.Context, location Location, ttl time.Duration) error {
	key := fmt.Sprintf("courier:%s:latest", location.CourierID)
	payload, err := json.Marshal(location)
	if err != nil {
		return err
	}
	pipe := c.client.Pipeline()
	pipe.Set(ctx, key, payload, ttl)
	pipe.GeoAdd(ctx, "couriers:geo", &redis.GeoLocation{
		Name:      location.CourierID,
		Longitude: location.Lng,
		Latitude:  location.Lat,
	})
	pipe.Expire(ctx, "couriers:geo", ttl)
	_, err = pipe.Exec(ctx)
	return err
}

func (c *Client) GetLatest(ctx context.Context, courierID string) (Location, error) {
	key := fmt.Sprintf("courier:%s:latest", courierID)
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		return Location{}, err
	}
	var loc Location
	if err := json.Unmarshal([]byte(val), &loc); err != nil {
		return Location{}, err
	}
	return loc, nil
}

func (c *Client) AddStreamEvent(ctx context.Context, location Location) (string, error) {
	values := map[string]interface{}{
		"courier_id":  location.CourierID,
		"lat":         fmt.Sprintf("%f", location.Lat),
		"lng":         fmt.Sprintf("%f", location.Lng),
		"accuracy_m":  fmt.Sprintf("%f", location.AccuracyM),
		"recorded_at": location.RecordedAt.Format(time.RFC3339Nano),
		"source":      location.Source,
		"event_id":    location.EventID,
	}
	if len(location.Metadata) > 0 {
		meta, _ := json.Marshal(location.Metadata)
		values["metadata"] = string(meta)
	}
	return c.client.XAdd(ctx, &redis.XAddArgs{
		Stream: c.stream,
		Values: values,
	}).Result()
}

func (c *Client) Nearby(ctx context.Context, lat, lng float64, radiusM float64, limit int64) ([]redis.GeoLocation, error) {
	return c.client.GeoRadius(ctx, "couriers:geo", lng, lat, &redis.GeoRadiusQuery{
		Radius:    radiusM,
		Unit:      "m",
		Count:     limit,
		WithCoord: true,
	}).Result()
}

func (c *Client) Client() *redis.Client {
	return c.client
}
