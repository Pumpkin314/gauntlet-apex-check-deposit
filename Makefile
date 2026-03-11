.PHONY: dev down logs test test-e2e migrate seed reset

dev:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f

test:
	go test ./... -v

test-e2e:
	cd web && npx playwright test

migrate:
	docker compose exec api /api migrate

seed:
	docker compose exec -T postgres psql -U apex -d apex_check_deposit < db/seed.sql

reset:
	docker compose exec -T postgres psql -U apex -d apex_check_deposit -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
	$(MAKE) dev
