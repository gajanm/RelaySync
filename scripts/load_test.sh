#!/usr/bin/env bash
set -euo pipefail

API_URL=${API_URL:-"http://localhost:8080"}
API_KEY=${API_KEY:-"local-dev-key"}
COURIER_ID=${COURIER_ID:-"loadtest-courier"}
TOTAL=${TOTAL:-500}
CONCURRENCY=${CONCURRENCY:-20}

create_resp=$(curl -s -X POST "$API_URL/couriers" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d '{"id":"'$COURIER_ID'","name":"Load Test Courier","status":"active"}')

echo "Using courier: $COURIER_ID"

seq 1 "$TOTAL" | xargs -P "$CONCURRENCY" -I {} bash -c '
  lat=$(python - <<PY
import random
print(37.770 + random.random()/500)
PY
)
  lng=$(python - <<PY
import random
print(-122.410 + random.random()/500)
PY
)
  curl -s -X POST "$API_URL/couriers/'$COURIER_ID'/location" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: '$API_KEY'" \
    -d "{\"lat\":$lat,\"lng\":$lng,\"accuracy_m\":4}" > /dev/null
'

echo "Load test complete: $TOTAL pings." 
