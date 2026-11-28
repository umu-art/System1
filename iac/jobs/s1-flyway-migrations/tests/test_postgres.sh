#!/usr/bin/env bash
set -euo pipefail

psql() {
  docker exec "$POSTGRES_CONTAINER" psql -U postgres -d system1 -v ON_ERROR_STOP=1 "$@"
}

psql -c "CREATE ROLE flyway LOGIN PASSWORD 'test-flyway';" \
  -c 'GRANT CONNECT, CREATE ON DATABASE system1 TO flyway;' \
  -c 'GRANT USAGE, CREATE ON SCHEMA public TO flyway;'

migrate() {
  docker run --rm --network host \
    -e "FLYWAY_URL=jdbc:postgresql://127.0.0.1:${POSTGRES_PORT}/system1" \
    -e FLYWAY_USER=flyway -e FLYWAY_PASSWORD=test-flyway \
    "$IMAGE" migrate
}

migrate
history='SELECT count(*) FROM public.flyway_schema_history WHERE success AND type = '\''SQL'\'';'
expected=$(find db/postgres/migrations -maxdepth 1 -type f -name 'V*.sql' -printf '.' | wc -c)
(( expected > 0 ))
test "$(psql -Atc "$history")" -eq "$expected"
migrate
test "$(psql -Atc "$history")" -eq "$expected"
