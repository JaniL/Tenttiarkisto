# Deploying Tenttiarkisto to Scaleway (serverless)

This is the deploy runbook. Application development (Go, tests, image
builds) happens on the dev machine; everything below that talks to
Scaleway happens on the **deploy machine**.

## Stack

| Component     | Service                                    |
| ------------- | ------------------------------------------ |
| App runtime   | Scaleway Serverless Containers (fr-par)   |
| Database      | Scaleway Serverless SQL DB (PostgreSQL, scales to zero) |
| Exam files    | Scaleway Object Storage (private bucket, S3 API) |
| Image         | Scaleway Container Registry               |
| Infra         | Terraform in `infra/`                     |

## Prerequisites (deploy machine)

- `podman` (or set `CONTAINER_RUNTIME=docker make ...`)

Note on cross-builds: `make image` builds for `$(PLATFORM)` (default
`linux/amd64`, which Scaleway requires). The Dockerfile passes the
platform via the standard `TARGETARCH` build arg, so `make image
PLATFORM=linux/arm64` builds native on Apple Silicon for local
testing. On Apple Silicon podman machines with broken amd64 emulation
(via Rosetta/QEMU), build amd64 images on an amd64 machine (or use
`docker buildx` with a working emulation setup) — the image must be
amd64 for Scaleway.
- `terraform` >= 1.6
- Scaleway credentials: `SCW_ACCESS_KEY`, `SCW_SECRET_KEY`,
  `SCW_DEFAULT_PROJECT_ID` (or pass `-var project_id=...`).
  Create keys in the Scaleway console under IAM (an *API key* with
  permissions for the project, or your personal key).
- Network access to `ftp.ripe.net` (only for `make geo-update`).

## First deploy

```sh
git clone https://github.com/JaniL/tenttiarkisto && cd tenttiarkisto
make geo-update          # embed fresh Finnish prefixes (optional but recommended)
make test                # go test ./...
make image               # podman build (linux/amd64)
make tf-init              # terraform init (infra/)

# Log in to the registry with your Scaleway API key:
podman login rg.fr-par.scw.cloud -u $SCW_ACCESS_KEY -p $SCW_SECRET_KEY

make push                 # push image :$(git rev-parse --short HEAD)
make tf-plan              # plan with image_tag = current SHA
make tf-apply             # create DB, bucket, IAM, container

# Create the super admin (run once, from this repo):
make run-migrate DATABASE_URL="$(terraform -chdir=infra output -raw database_endpoint)"  # already applied by container on boot, but idempotent
make run-seed   DATABASE_URL="..." # optional example subjects
go run ./cmd/migrate -admin-email you@example.fi -admin-password '...' -admin-org 'TKO-äly ry' # super admin
```

Note: `terraform output -raw database_endpoint` prints a
`postgres://...` URL; the container also runs migrations at startup,
so `run-migrate` is usually unnecessary. Use it only for debugging.

The container endpoint:

```sh
terraform -chdir=infra output container_endpoint
```

## Verify the client IP header (one-time)

The geo gate trusts the header named by
`TRUSTED_FORWARDED_IP_HEADER` (default `X-Forwarded-For`). Confirm
what the Scaleway edge actually sets before relying on it:

```sh
curl -s https://<container-endpoint>/healthz   # 200 ok
# temporarily set TRUSTED_FORWARDED_IP_HEADER=Debug-Log in infra/container.tf,
# or check the app logs: each blocked request logs ip/method/path.
```

From a Finnish IP:

- `GET /login` -> 200
- `POST /login` etc. work.

From a non-Finnish IP (VPN abroad, `curl --interface`, etc.):

- `GET /login` -> **403** (empty body)
- `POST /exams` -> **403**
- `GET /exams` -> 200 (reads stay open)

If the header is wrong, adjust `TRUSTED_FORWARDED_IP_HEADER` in
`infra/container.tf` and `make tf-apply`. The gate reads the first
(leftmost) entry of the header value.

## Deploy a new version

```sh
git pull
make geo-update    # if you want fresh prefix data in the image
make test
make image
make push
make tf-plan       # image_tag change triggers a container redeploy
make tf-apply
```

## Smoke test after each deploy

1. `GET /healthz` -> `ok` (also the liveness probe path).
2. Log in as super admin, add a Subject, add a Course under it.
3. Upload an exam: 2-3 JPEG/PNG images -> check the exam page renders
   and `GET /exams/{id}/file` streams the merged PDF
   (`Content-Type: application/pdf`).
4. Approve the exam in `/approvelisting`; it appears on `/exams`.
5. New user registers, gets "no rights" on other subjects' courses.
6. From a non-FI IP: writes and `/login` return 403; reads work.

## Costs (rough)

- Serverless SQL DB: min billing 1 GB (~€0.10/mo) + vCPU/GB used.
- Containers: per-request; min_scale=0 means zero cost when idle.
- Object storage: ~€0.01/GB/mo. Bucket starts empty (fresh start;
  no legacy data migration by design).

## Rollback

```sh
make tf-plan VERSION=<previous-sha> && make tf-apply
```

(Terraform state is local on the deploy machine — `infra/*.tfstate`.
Keep a backup of the state file; secrets are in it, store it safely.)

## Teardown

```sh
terraform -chdir=infra destroy
```

Buckets with objects need manual confirmation/emptying.