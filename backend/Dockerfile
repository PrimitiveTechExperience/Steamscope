# syntax=docker/dockerfile:1
#
# Backend image: the API server plus the scraper and backfill tools.
# Build from the REPOSITORY ROOT (go.mod lives there):
#   docker build -f backend/Dockerfile -t steamscope-api --build-arg VERSION=$(git rev-parse --short HEAD) .

FROM golang:1.27-alpine AS build
WORKDIR /src

# Download modules first so this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY backend ./backend
ARG VERSION=dev
ENV CGO_ENABLED=0
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    for cmd in server scraper backfill-history; do \
      go build -trimpath \
        -ldflags "-s -w -X github.com/PrimitiveTechExperience/Steamscope/backend/internal/observability.Version=${VERSION}" \
        -o /out/${cmd} ./backend/cmd/${cmd}; \
    done

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/ /app/
# The scraper reads Steam cookies from this path (STEAM_COOKIE_FILE_PATH).
COPY backend/internal/config/config.json /app/internal/config/config.json
USER app

ENV PORT=8080
EXPOSE 8080
# /api/health is the cheap liveness probe; /api/ready also checks Postgres and Redis.
HEALTHCHECK --interval=30s --timeout=3s --start-period=20s --retries=3 \
  CMD wget -qO- http://127.0.0.1:${PORT}/api/health >/dev/null || exit 1

# Run the daily scrape instead with:  docker run ... /app/scraper
CMD ["/app/server"]
