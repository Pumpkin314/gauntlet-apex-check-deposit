#!/bin/sh

# Wait for database to be ready (Cloud SQL proxy may take a few seconds)
echo "Waiting for database..."
retries=0
until psql "$DATABASE_URL" -c "SELECT 1" > /dev/null 2>&1; do
  retries=$((retries + 1))
  if [ "$retries" -ge 30 ]; then
    echo "Database not ready after 30 attempts, proceeding anyway..."
    break
  fi
  echo "  attempt $retries/30..."
  sleep 2
done

echo "Running migrations..."
goose -dir /migrations postgres "$DATABASE_URL" up || echo "WARNING: migrations failed, continuing..."

echo "Running seed..."
if [ -f /seed.sql ]; then
  psql "$DATABASE_URL" < /seed.sql 2>/dev/null || echo "WARNING: seed failed or already applied, continuing..."
fi

echo "Starting API server..."
exec /api
