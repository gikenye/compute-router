# Multi-stage build — static Go binary + static web UI assets, small
# runtime image. Build locally with: docker build -t compute-router-core .
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod ./
# go.sum intentionally not committed from this environment (no network
# access here to generate one) — `make deps` locally produces it.
COPY go.sum* ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /server /app/server
COPY web ./web
EXPOSE 8080
ENTRYPOINT ["/app/server"]
