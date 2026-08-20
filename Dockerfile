# Multi-stage build for OpenOutreach outreachd (hosted cold-cli control plane).
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/outreachd ./cmd/outreachd

FROM debian:bookworm-slim
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl \
  && rm -rf /var/lib/apt/lists/* \
  && useradd --system --uid 65532 --create-home nonroot
COPY --from=build /out/outreachd /outreachd
USER nonroot:nonroot
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8080/internal/health || exit 1
ENTRYPOINT ["/outreachd"]
