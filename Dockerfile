FROM golang:1.25.0 AS builder
WORKDIR /src/shop-gateway-service

COPY shop-gateway-service/go.mod shop-gateway-service/go.sum ./
COPY shop-contracts /src/shop-contracts
RUN go mod download

COPY shop-gateway-service ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/gateway-service ./cmd/gateway

FROM alpine:3.20
WORKDIR /app

RUN addgroup -S app && adduser -S app -G app

COPY --from=builder /out/gateway-service /app/gateway-service
COPY shop-gateway-service/config/config.docker.yaml /app/config/config.docker.yaml
# Сертификаты приходят через volume ../infra/certs:/app/certs (см. compose), в образ не вкапываются.
COPY shop-gateway-service/web ./web
COPY shop-gateway-service/openapi ./openapi


USER app

EXPOSE 8083

ENV CONFIG_PATH=/app/config/config.docker.yaml

ENTRYPOINT ["/app/gateway-service"]
