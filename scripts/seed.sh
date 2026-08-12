#!/usr/bin/env bash
set -euo pipefail

COMPOSE=${COMPOSE:-"docker compose"}

$COMPOSE exec -T postgres psql -U relaysync -d relaysync <<'SQL'
INSERT INTO couriers (id, name, status)
VALUES
  ('courier-seed-1', 'Seed Courier 1', 'active'),
  ('courier-seed-2', 'Seed Courier 2', 'active'),
  ('courier-seed-3', 'Seed Courier 3', 'inactive')
ON CONFLICT (id) DO NOTHING;

INSERT INTO subscriptions (courier_id, topic)
VALUES
  ('courier-seed-1', 'courier-courier-seed-1'),
  ('courier-seed-2', 'courier-courier-seed-2')
ON CONFLICT DO NOTHING;
SQL

echo "Seeded couriers and subscriptions."
