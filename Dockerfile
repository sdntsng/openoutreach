# Multi-stage build for OpenOutreach outreachd (hosted cold-cli control plane).
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/outreachd ./cmd/outreachd

FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
  && adduser -D -u 65532 nonroot
COPY --from=build /out/outreachd /outreachd
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/outreachd"]
