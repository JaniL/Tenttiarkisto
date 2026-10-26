# Tenttiarkisto

Exam archive for the Student Organizations of the Student Union of the
University of Helsinki (HYY). Originally a Ruby on Rails 4 application
(2015, preserved under the `legacy-rails` git tag); rewritten in Go for
Scaleway serverless deployment.

Live history: TKO-äly ry later replaced this app with
[Tärpistö](https://github.com/TKOaly/exam-archive-new). This repo is a
personal modernization of the original 2015 codebase.

## Stack

- **Go 1.27** app: `chi` router, `html/template`, `pgx/v5`,
  `alexedwards/scs` (PostgreSQL session store), `gorilla/csrf`,
  `bcrypt` auth
- **Scaleway Serverless Containers** (fr-par), **Serverless SQL
  Database** (PostgreSQL, scales to zero), **Object Storage** (S3 API)
- **Terraform** infra in [`infra/`](infra/)
- **Geo gate**: writes (`POST/PUT/PATCH/DELETE`) and the login/register
  pages are blocked for non-Finnish client IPs (403, empty body),
  using Finnish prefixes embedded at build time from the RIPE NCC
  delegated statistics. Feature-flagged via `GEO_GATE_ENABLED`.

## Development

```sh
make test        # go test ./...
make vet         # go vet ./...
make fmt         # gofmt everything
make build       # go build ./...
make geo-update  # regenerate Finnish prefix data (network)
make image       # build the container image (podman, linux/amd64)
```

Run locally against a Postgres and any S3-compatible storage:

```sh
export PORT=8080
export DATABASE_URL='postgres://localhost:5432/tenttiarkisto?sslmode=disable'
export SESSION_SECRET='<32+ bytes>'
export S3_ENDPOINT='http://localhost:9000' S3_REGION=fr-par S3_BUCKET=tenttiarkisto-exams
export S3_ACCESS_KEY=... S3_SECRET_KEY=...
export GEO_GATE_ENABLED=false            # disable the Finland-only gate locally
export SESSION_COOKIE_SECURE=false      # for plain-http local testing
go run ./cmd/server                      # migrations run automatically at boot
```

Deployment (on the deploy machine): see [docs/DEPLOY.md](docs/DEPLOY.md).

## Layout

```
cmd/server            application entrypoint
cmd/migrate           migrations / seeds / super-admin bootstrap
internal/config       env configuration
internal/db           pool + embedded SQL migrations
internal/models       domain types, queries, permissions (ported from Rails)
internal/geogate      Finland-only middleware + embedded prefixes
internal/pdfmerge     images -> single PDF (replaces ImageMagick)
internal/storage      S3-compatible object storage client
internal/web           routes, handlers, templates, static assets
infra/                 Terraform (Scaleway)
scripts/genfi          RIPE delegated-stats -> prefix data generator
```

## License

As the original project: unlicensed legacy university code. The Go
rewrite by JaniL is provided as-is.