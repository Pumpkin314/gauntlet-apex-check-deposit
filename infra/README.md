# Apex Check Deposit — Infrastructure

Pulumi Go program that provisions the GCP infrastructure for the Apex Mobile Check Deposit system.

## Stack topology

| Resource | dev | prod |
|---|---|---|
| Cloud Run — API | scale-to-zero (min=0) | min=1 |
| Cloud Run — VSS stub | scale-to-zero (min=0) | min=1 |
| Cloud Run — Settlement stub | scale-to-zero (min=0) | min=1 |
| Cloud SQL (Postgres 16) | `db-f1-micro`, no backups | `db-g1-small`, backups enabled |
| GCS bucket | `apex-deposits-<project>` | `apex-deposits-<project>` |
| Artifact Registry | `apex-check-deposit` (DOCKER) | same repo |
| Secret Manager | — | `JWT_SECRET`, `SETTLEMENT_BANK_TOKEN` |

## Prerequisites

- [Pulumi CLI](https://www.pulumi.com/docs/install/) installed
- `gcloud` authenticated with a project that has the following APIs enabled:
  - `run.googleapis.com`
  - `sqladmin.googleapis.com`
  - `storage.googleapis.com`
  - `artifactregistry.googleapis.com`
  - `secretmanager.googleapis.com` (prod only)
- Docker installed and authenticated to Artifact Registry:
  ```bash
  gcloud auth configure-docker us-central1-docker.pkg.dev
  ```

## Setup

```bash
cd infra
go mod tidy           # resolve dependencies
pulumi login          # authenticate to Pulumi backend (or use --local)
```

## Deploy dev

```bash
make deploy-dev GCP_PROJECT=your-gcp-project-id
```

This will:
1. Build multi-stage Docker images for api, vss, and settlement
2. Push images to Artifact Registry (`us-central1-docker.pkg.dev/<project>/apex-check-deposit/`)
3. Run `pulumi up --stack dev`

After deploy, get the API URL:
```bash
cd infra && pulumi stack output apiUrl --stack dev
```

## Deploy prod

```bash
make deploy-prod GCP_PROJECT=your-gcp-prod-project-id
```

After the first prod deploy, **replace the placeholder secret values** before routing traffic:
```bash
echo -n "your-real-jwt-secret" | \
  gcloud secrets versions add JWT_SECRET --data-file=- --project=your-gcp-prod-project-id

echo -n "your-real-settlement-token" | \
  gcloud secrets versions add SETTLEMENT_BANK_TOKEN --data-file=- --project=your-gcp-prod-project-id
```

## Stack differences

| Config key | dev | prod |
|---|---|---|
| `minInstances` | 0 (scale to zero) | 1 (always warm) |
| `sqlTier` | `db-f1-micro` | `db-g1-small` |
| `sqlBackups` | false | true |
| `enableSecrets` | false (env vars) | true (Secret Manager) |

## Outputs

| Output | Description |
|---|---|
| `apiUrl` | Cloud Run URL for the Go API |
| `vssUrl` | Cloud Run URL for the VSS stub |
| `settlementUrl` | Cloud Run URL for the Settlement stub |
| `dbConnectionName` | Cloud SQL connection name (`project:region:instance`) |
| `jwtSecretName` | Secret Manager resource name for JWT_SECRET (prod only) |
| `settlementTokenSecretName` | Secret Manager resource name for SETTLEMENT_BANK_TOKEN (prod only) |

## Independence from local dev

`make dev` (Docker Compose) runs entirely locally and has no dependency on Pulumi or GCP.
The two environments share the same container images but have independent state.
Destroying the dev GCP stack does not affect the prod stack, and vice versa.
