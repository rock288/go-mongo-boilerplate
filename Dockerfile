# syntax=docker/dockerfile:1.7
ARG GO_VERSION=1.26
ARG TARGET=server

FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG TARGET
RUN CGO_ENABLED=0 GOOS=linux \
    go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/app ./cmd/${TARGET}

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=builder /out/app /app/app
COPY --from=builder /src/config /app/config
ENV CONFIG_PATH=/app/config/config.yaml
USER nonroot:nonroot
EXPOSE 8002
ENTRYPOINT ["/app/app"]
