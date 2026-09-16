#!/bin/sh
# Disposable PostgreSQL integration test; never reads application credentials.
set -eu
name="mch-postgres-test-$$"
network="mch-test-net-$$"
docker network create "$network" >/dev/null
cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; docker network rm "$network" >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM
docker run -d --rm --name "$name" --network "$network" -e POSTGRES_PASSWORD=local-test -e POSTGRES_DB=clubtest postgres:17-alpine >/dev/null
for attempt in $(seq 1 30); do
 if docker exec "$name" pg_isready -U postgres -d clubtest >/dev/null 2>&1; then break; fi
 sleep 1
done
docker build --target toolchain -t maceclubheemskerk:pg-tests .
docker run --rm --network "$network" -e "MCH_TEST_POSTGRES=postgres://postgres:local-test@$name:5432/clubtest?sslmode=disable" maceclubheemskerk:pg-tests go test ./internal/content -run '^TestManagementAndMigration$' -v
