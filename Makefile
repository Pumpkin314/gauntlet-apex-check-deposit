.PHONY: dev down logs test test-e2e migrate seed reset deploy-dev deploy-prod

# ── Deploy config ────────────────────────────────────────────────────────────
# Usage: make deploy-dev GCP_PROJECT=your-gcp-project-id
#        make deploy-prod GCP_PROJECT=your-gcp-project-id
GCP_PROJECT ?= $(error GCP_PROJECT is required, e.g.: make deploy-dev GCP_PROJECT=my-project)
REGION      ?= us-central1
TAG         ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo latest)
REGISTRY     = $(REGION)-docker.pkg.dev/$(GCP_PROJECT)/apex-check-deposit

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

# ── Cloud deploy targets ─────────────────────────────────────────────────────
deploy-dev:
	docker build -t $(REGISTRY)/api:$(TAG)        -f cmd/api/Dockerfile .
	docker build -t $(REGISTRY)/vss:$(TAG)        -f cmd/vendor-stub/Dockerfile .
	docker build -t $(REGISTRY)/settlement:$(TAG) -f cmd/settlement-stub/Dockerfile .
	docker push $(REGISTRY)/api:$(TAG)
	docker push $(REGISTRY)/vss:$(TAG)
	docker push $(REGISTRY)/settlement:$(TAG)
	cd infra && pulumi stack select dev --create
	cd infra && pulumi config set gcp:project $(GCP_PROJECT) --stack dev
	cd infra && pulumi config set apex-check-deposit:apiTag $(TAG) --stack dev
	cd infra && pulumi config set apex-check-deposit:vssTag $(TAG) --stack dev
	cd infra && pulumi config set apex-check-deposit:settlementTag $(TAG) --stack dev
	cd infra && pulumi up --stack dev --yes

deploy-prod:
	docker build -t $(REGISTRY)/api:$(TAG)        -f cmd/api/Dockerfile .
	docker build -t $(REGISTRY)/vss:$(TAG)        -f cmd/vendor-stub/Dockerfile .
	docker build -t $(REGISTRY)/settlement:$(TAG) -f cmd/settlement-stub/Dockerfile .
	docker push $(REGISTRY)/api:$(TAG)
	docker push $(REGISTRY)/vss:$(TAG)
	docker push $(REGISTRY)/settlement:$(TAG)
	cd infra && pulumi stack select prod --create
	cd infra && pulumi config set gcp:project $(GCP_PROJECT) --stack prod
	cd infra && pulumi config set apex-check-deposit:apiTag $(TAG) --stack prod
	cd infra && pulumi config set apex-check-deposit:vssTag $(TAG) --stack prod
	cd infra && pulumi config set apex-check-deposit:settlementTag $(TAG) --stack prod
	cd infra && pulumi up --stack prod --yes
