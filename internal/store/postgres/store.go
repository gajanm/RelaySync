package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

type Courier struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type LocationEvent struct {
	EventID    string            `json:"event_id"`
	CourierID  string            `json:"courier_id"`
	Lat        float64           `json:"lat"`
	Lng        float64           `json:"lng"`
	AccuracyM  float64           `json:"accuracy_m"`
	RecordedAt time.Time         `json:"recorded_at"`
	Source     string            `json:"source"`
	Metadata   map[string]string `json:"metadata"`
}

type Subscription struct {
	ID        string    `json:"id"`
	CourierID string    `json:"courier_id"`
	Topic     string    `json:"topic"`
	CreatedAt time.Time `json:"created_at"`
}

func New(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) CreateCourier(ctx context.Context, courier Courier) (Courier, error) {
	row := s.pool.QueryRow(ctx, `INSERT INTO couriers (id, name, status) VALUES ($1, $2, $3) RETURNING id, name, status, created_at`, courier.ID, courier.Name, courier.Status)
	var out Courier
	if err := row.Scan(&out.ID, &out.Name, &out.Status, &out.CreatedAt); err != nil {
		return Courier{}, err
	}
	return out, nil
}

func (s *Store) GetCourier(ctx context.Context, id string) (Courier, error) {
	row := s.pool.QueryRow(ctx, `SELECT id, name, status, created_at FROM couriers WHERE id = $1`, id)
	var out Courier
	if err := row.Scan(&out.ID, &out.Name, &out.Status, &out.CreatedAt); err != nil {
		return Courier{}, err
	}
	return out, nil
}

func (s *Store) ListCouriers(ctx context.Context, limit, offset int) ([]Courier, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, status, created_at FROM couriers ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Courier
	for rows.Next() {
		var c Courier
		if err := rows.Scan(&c.ID, &c.Name, &c.Status, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CreateSubscription(ctx context.Context, sub Subscription) (Subscription, error) {
	row := s.pool.QueryRow(ctx, `INSERT INTO subscriptions (courier_id, topic) VALUES ($1, $2) RETURNING id, courier_id, topic, created_at`, sub.CourierID, sub.Topic)
	var out Subscription
	if err := row.Scan(&out.ID, &out.CourierID, &out.Topic, &out.CreatedAt); err != nil {
		return Subscription{}, err
	}
	return out, nil
}

func (s *Store) DeleteSubscription(ctx context.Context, id string) error {
	cmd, err := s.pool.Exec(ctx, `DELETE FROM subscriptions WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("not found")
	}
	return nil
}

func (s *Store) ListSubscriptions(ctx context.Context, courierID string) ([]Subscription, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, courier_id, topic, created_at FROM subscriptions WHERE courier_id = $1 ORDER BY created_at DESC`, courierID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		var sub Subscription
		if err := rows.Scan(&sub.ID, &sub.CourierID, &sub.Topic, &sub.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *Store) InsertLocationEvents(ctx context.Context, events []LocationEvent) error {
	if len(events) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, e := range events {
		metaBytes, err := json.Marshal(e.Metadata)
		if err != nil {
			return err
		}
		batch.Queue(`INSERT INTO location_events (event_id, courier_id, lat, lng, accuracy_m, recorded_at, source, metadata)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (event_id) DO NOTHING`,
			e.EventID, e.CourierID, e.Lat, e.Lng, e.AccuracyM, e.RecordedAt, e.Source, metaBytes,
		)
	}
	results := s.pool.SendBatch(ctx, batch)
	defer results.Close()
	for i := 0; i < len(events); i++ {
		if _, err := results.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetLocationHistory(ctx context.Context, courierID string, from, to time.Time, limit int) ([]LocationEvent, error) {
	rows, err := s.pool.Query(ctx, `SELECT event_id, courier_id, lat, lng, accuracy_m, recorded_at, source, metadata FROM location_events WHERE courier_id = $1 AND recorded_at BETWEEN $2 AND $3 ORDER BY recorded_at DESC LIMIT $4`, courierID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LocationEvent
	for rows.Next() {
		var e LocationEvent
		var metaBytes []byte
		if err := rows.Scan(&e.EventID, &e.CourierID, &e.Lat, &e.Lng, &e.AccuracyM, &e.RecordedAt, &e.Source, &metaBytes); err != nil {
			return nil, err
		}
		if len(metaBytes) > 0 {
			_ = json.Unmarshal(metaBytes, &e.Metadata)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
