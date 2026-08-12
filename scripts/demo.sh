#!/usr/bin/env bash
set -euo pipefail

API_URL=${API_URL:-"http://localhost:8080"}
API_KEY=${API_KEY:-"local-dev-key"}

create_resp=$(curl -s -X POST "$API_URL/couriers" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d '{"name":"Demo Courier","status":"active"}')

export CREATE_RESP="$create_resp"

courier_id=$(python - <<'PY'
import json, os
resp = os.environ['CREATE_RESP']
print(json.loads(resp)['id'])
PY
)

echo "Created courier: $courier_id"

curl -s -X POST "$API_URL/subscriptions" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d '{"courier_id":"'$courier_id'"}' > /dev/null

echo "Sending location pings..."
for i in $(seq 1 60); do
  lat=$(python - <<PY
import random
print(37.775 + random.random()/1000)
PY
)
  lng=$(python - <<PY
import random
print(-122.418 + random.random()/1000)
PY
)
  curl -s -X POST "$API_URL/couriers/$courier_id/location" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: $API_KEY" \
    -d '{"lat":'$lat',"lng":'$lng',"accuracy_m":5}' > /dev/null
  sleep 0.02
done

echo "Latest location:"
curl -s "$API_URL/couriers/$courier_id/location/latest" -H "X-API-Key: $API_KEY" | sed 's/^/  /'

echo "\nHistory (last 10):"
now=$(python - <<'PY'
import datetime
print(datetime.datetime.utcnow().isoformat() + 'Z')
PY
)
from=$(python - <<'PY'
import datetime
print((datetime.datetime.utcnow() - datetime.timedelta(minutes=5)).isoformat() + 'Z')
PY
)

curl -s "$API_URL/couriers/$courier_id/location/history?from=$from&to=$now&limit=10" -H "X-API-Key: $API_KEY" | sed 's/^/  /'

echo "\nNearby couriers:"
curl -s "$API_URL/couriers/nearby?lat=37.775&lng=-122.418&radius_m=500&limit=5" -H "X-API-Key: $API_KEY" | sed 's/^/  /'

echo "\nDemo complete. Check logs for mock notifications."
