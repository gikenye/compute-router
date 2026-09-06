# Multi-stage build — static Go binary + static web UI assets, small
# runtime image. Build from the repository root with:
# docker build -f services/core/Dockerfile -t compute-router-core .
FROM golang:1.25 AS build
WORKDIR /src
COPY services/core/go.mod ./
# go.sum intentionally not committed from this environment (no network
# access here to generate one) — `make deps` locally produces it.
COPY services/core/go.sum* ./
RUN go mod download || true
COPY services/core/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /server /app/server
COPY services/core/web ./web
EXPOSE 8080
ENTRYPOINT ["/app/server"]
