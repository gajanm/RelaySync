# RelaySync

RelaySync is a real-time courier location tracking gateway that synchronizes courier coordinates across distributed mobile clients. It ingests high-frequency GPS pings, caches live locations in Redis, persists history in PostgreSQL, and fans out notifications via Firebase Cloud Messaging (FCM) or a mock notifier.

## Overview

RelaySync is designed for high-volume ingestion (1–5 pings/sec per courier) while keeping the request path fast. Redis is used for the hot-path cache and Redis Streams provide an internal event pipeline for batched persistence and notification fanout.

### Architecture

```
Mobile Clients -> REST API -> Redis (latest + geo + stream)
                                 |           |
                                 |           v
                                 |      Stream Consumer -> Postgres (history)
                                 |                         |
                                 |                         v
                                 +----> Notifier (FCM or mock)
```

Key characteristics:
- Redis stores the latest courier location with TTL and provides GEO queries.
- Redis Streams buffer updates; a consumer group batches inserts to Postgres.
- Notification fanout is async via worker pool (mocked locally if FCM disabled).

## Data Model

PostgreSQL schema (see `migrations/001_init.sql`):
- `couriers(id, name, status, created_at)`
- `location_events(event_id, courier_id, lat, lng, recorded_at, source, metadata)`
- `subscriptions(id, courier_id, topic, created_at)`

Indexes:
- `location_events (courier_id, recorded_at desc)` for history queries.
- `subscriptions (courier_id)` for watcher lookups.

## API Contract (highlights)

All endpoints return JSON and most are protected by API key: `X-API-Key: <API_KEY>`.

### Create courier
```
POST /couriers
{
  "name": "Courier A",
  "status": "active"
}
```

### Ingest location ping
```
POST /couriers/{id}/location
{
  "lat": 37.775,
  "lng": -122.418,
  "accuracy_m": 5,
  "timestamp": "2024-03-10T12:00:00Z"
}
```
- Writes latest location to Redis (TTL)
- Adds entry to Redis Stream
- Responds `202` quickly with server time

### Get latest location (Redis)
```
GET /couriers/{id}/location/latest
```

### Get history (Postgres)
```
GET /couriers/{id}/location/history?from=...&to=...&limit=100
```

### Spatial query (Redis GEO)
```
GET /couriers/nearby?lat=37.775&lng=-122.418&radius_m=500&limit=10
```

### Subscriptions
```
POST /subscriptions
{
  "courier_id": "<id>",
  "fcm_topic": "courier-<id>"
}
```

Full OpenAPI spec is served at `/openapi.yaml` and Swagger UI at `/docs`.

## Local Development

### Prereqs
- Docker + Docker Compose

### Quick start
```
make up
```

Optional: copy `.env.example` to `.env` to override defaults.

The API will be available at `http://localhost:8080`.

### Useful commands
```
make migrate   # run database migrations
make seed      # seed sample data
make demo      # create courier, send 50+ pings, show latest/history
make test      # run unit/integration tests
make load      # run basic load simulation
```

## Demo / Acceptance Checklist

1. `docker compose up --build`
2. `make demo`:
   - creates a courier
   - sends 60 rapid pings
   - reads latest from Redis
   - reads history from Postgres
   - queries nearby couriers
   - logs mock notifications

## Configuration

All config is via environment variables. See `.env.example` for defaults.

Notable vars:
- `API_KEY`: required for ingest and subscription endpoints.
- `FCM_ENABLED`: if `true`, uses FCM instead of mock notifier.
- `GOOGLE_APPLICATION_CREDENTIALS`: service account JSON for FCM.

### Enabling real FCM
1. Create a Firebase project and service account.
2. Set `FCM_ENABLED=true`, `FCM_PROJECT_ID=<project-id>`.
3. Export `GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account.json`.
4. Create a topic for a courier (e.g., `courier-<id>`).

## Scaling Notes

- Redis Streams decouple ingest from Postgres writes. The consumer batches inserts (size/time thresholds) to reduce write amplification.
- The notification queue is bounded to provide backpressure.
- Rate limiting is per API key and courier ID.
- For higher scale:
  - Increase stream consumer replicas with different consumer names.
  - Increase Redis and Postgres pool sizes.
  - Move notifications to a separate worker service.

## Observability

- Structured JSON logs with request IDs.
- Prometheus metrics at `/metrics`:
  - `ingest_requests_total`
  - `ingest_latency_seconds`
  - `redis_write_latency_seconds`
  - `stream_lag`
  - `batch_insert_size`
  - `notifications_sent_total`
  - `notifier_latency_seconds`

## Testing Strategy

- Unit tests for validation and pipeline parsing.
- Integration tests for Redis and Postgres via `REDIS_URL` and `POSTGRES_URL`.

## Operational Notes

- Health endpoints: `/healthz` (liveness), `/readyz` (dependencies).
- Requests enforce timeouts and basic validation.
- Ingest endpoints return `202 Accepted` to keep latency low.
