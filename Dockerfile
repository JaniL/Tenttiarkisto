# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
# TARGETARCH is set from --platform (amd64 for Scaleway)
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/tenttiarkisto ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/tenttiarkisto-migrate ./cmd/migrate

# ---- run stage ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

COPY --from=build --chown=nonroot:nonroot /out/tenttiarkisto /app/tenttiarkisto
COPY --from=build --chown=nonroot:nonroot /out/tenttiarkisto-migrate /app/tenttiarkisto-migrate

USER nonroot
EXPOSE 8080
ENTRYPOINT ["/app/tenttiarkisto"]