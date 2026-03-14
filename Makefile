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
	docker build --platform linux/amd64 -t $(REGISTRY)/api:$(TAG)        -f cmd/api/Dockerfile .
	docker build --platform linux/amd64 -t $(REGISTRY)/vss:$(TAG)        -f cmd/vendor-stub/Dockerfile .
	docker build --platform linux/amd64 -t $(REGISTRY)/settlement:$(TAG) -f cmd/settlement-stub/Dockerfile .
	docker push $(REGISTRY)/api:$(TAG)
	docker push $(REGISTRY)/vss:$(TAG)
	docker push $(REGISTRY)/settlement:$(TAG)
	cd infra && pulumi stack select dev --create
	cd infra && pulumi config set gcp:project $(GCP_PROJECT) --stack dev
	cd infra && pulumi config set apex-check-deposit:apiTag $(TAG) --stack dev
	cd infra && pulumi config set apex-check-deposit:vssTag $(TAG) --stack dev
	cd infra && pulumi config set apex-check-deposit:settlementTag $(TAG) --stack dev
	cd infra && pulumi up --stack dev --yes
	gcloud run services update $$(cd infra && pulumi stack output apiServiceName --stack dev) \
		--no-invoker-iam-check \
		--update-env-vars="SETTLEMENT_BANK_URL=$$(cd infra && pulumi stack output settlementUrl --stack dev)" \
		--region=$(REGION) --project=$(GCP_PROJECT) --quiet
	cd web && npm ci && VITE_SSE_URL=$$(cd ../infra && pulumi stack output apiUrl --stack dev) npm run build
	cd web && npx firebase-tools deploy --only hosting

deploy-prod:
	docker build --platform linux/amd64 -t $(REGISTRY)/api:$(TAG)        -f cmd/api/Dockerfile .
	docker build --platform linux/amd64 -t $(REGISTRY)/vss:$(TAG)        -f cmd/vendor-stub/Dockerfile .
	docker build --platform linux/amd64 -t $(REGISTRY)/settlement:$(TAG) -f cmd/settlement-stub/Dockerfile .
	docker push $(REGISTRY)/api:$(TAG)
	docker push $(REGISTRY)/vss:$(TAG)
	docker push $(REGISTRY)/settlement:$(TAG)
	cd infra && pulumi stack select prod --create
	cd infra && pulumi config set gcp:project $(GCP_PROJECT) --stack prod
	cd infra && pulumi config set apex-check-deposit:apiTag $(TAG) --stack prod
	cd infra && pulumi config set apex-check-deposit:vssTag $(TAG) --stack prod
	cd infra && pulumi config set apex-check-deposit:settlementTag $(TAG) --stack prod
	cd infra && pulumi up --stack prod --yes
	gcloud run services update $$(cd infra && pulumi stack output apiServiceName --stack prod) \
		--no-invoker-iam-check \
		--update-env-vars="SETTLEMENT_BANK_URL=$$(cd infra && pulumi stack output settlementUrl --stack prod)" \
		--region=$(REGION) --project=$(GCP_PROJECT) --quiet
	cd web && npm ci && VITE_SSE_URL=$$(cd ../infra && pulumi stack output apiUrl --stack prod) npm run build
	cd web && npx firebase-tools deploy --only hosting
