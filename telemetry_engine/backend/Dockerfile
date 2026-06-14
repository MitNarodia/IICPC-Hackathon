# Multi-service build: one Dockerfile produces any of the five Track 3 binaries.
# Select which with `--build-arg SERVICE=<name>` where name is a directory under
# cmd/ (ingestion-service, stream-processor, validation-engine, scoring-engine,
# leaderboard-service). docker-compose passes this per service.
#
# Stage 1 compiles a static binary; stage 2 is distroless (no shell, no libc) so
# the runtime image is tiny and has a minimal attack surface.

FROM golang:1.22-alpine AS build
WORKDIR /src

# Copy the whole module, resolve deps, build. We copy everything before
# `go mod download` because go.sum is generated here (it is not committed).
COPY . .
RUN go mod download

ARG SERVICE
RUN test -n "$SERVICE" || (echo "SERVICE build-arg is required" && false)
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath -ldflags="-s -w" \
        -o /out/app ./cmd/${SERVICE}

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /
COPY --from=build /out/app /app
# Distroless nonroot UID. Services listen on high ports (>1024) so no root need.
USER nonroot:nonroot
ENTRYPOINT ["/app"]
