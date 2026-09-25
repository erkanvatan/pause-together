# syntax=docker/dockerfile:1

# Web build: SvelteKit static SPA.
FROM node:24-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Dev and test image: Go plus air for hot reload. compose.dev.yml mounts the repo over /src.
# Debian's ffmpeg, the same 7.1 as the runtime image: both are Debian trixie.
FROM golang:1.27 AS go-dev
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg && rm -rf /var/lib/apt/lists/*
RUN go install github.com/air-verse/air@v1.67.4
COPY --from=golangci/golangci-lint:v2.14.0 /usr/bin/golangci-lint /usr/local/bin/
WORKDIR /src

# Go build with the web build embedded.
FROM golang:1.27 AS go
WORKDIR /src
COPY go.* ./
RUN go mod download
# Only Go sources, so edits to docs or Svelte files keep this layer cached.
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/
COPY --from=web /src/web/build ./web/build
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/pausetogether ./cmd/pausetogether

FROM debian:trixie-slim
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg && rm -rf /var/lib/apt/lists/*
COPY --from=go /out/pausetogether /usr/local/bin/pausetogether
EXPOSE 8080 8081
ENTRYPOINT ["/usr/local/bin/pausetogether"]
