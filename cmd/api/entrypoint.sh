#!/bin/sh
set -e

echo "Running migrations..."
goose -dir /migrations postgres "$DATABASE_URL" up

echo "Running seed..."
if [ -f /seed.sql ]; then
  psql "$DATABASE_URL" < /seed.sql
fi

echo "Starting API server..."
exec /api
