# shop-gateway

`shop-gateway` - HTTP API gateway для доступа к `shop-catalog-service` по gRPC.

Сервис принимает HTTP запросы, выполняет бизнес-валидацию в service-слое и проксирует чтение каталога в `catalog-service`.

## Что уже реализовано

1. HTTP endpoints:
   - `GET /products`
   - `GET /products/{id}`
   - `GET /health`
   - `GET /ready`
   - `GET /metrics`
   - `GET/POST /admin/log-level`
2. Слои:
   - `transport/http` (handlers + router)
   - `service/gateway` (business logic)
   - `adapters/catalog_grpc` (grpc adapter)
   - `client/grpc/catalog` (transport client)
3. Observability:
   - JSON logs (`slog`)
   - Prometheus metrics (`gateway_*`)
   - OpenTelemetry traces (HTTP + gRPC client)
4. Graceful shutdown и readiness check внешнего `catalog-service`.

## Архитектура

Поток запроса:

1. HTTP request приходит в `handlers/products.go`.
2. Handler вызывает `GatewayService`.
3. `GatewayService` валидирует вход и вызывает интерфейс `ProductRepository`.
4. `CatalogAdapter` реализует `ProductRepository` через `client/grpc/catalog`.
5. gRPC вызов уходит в `shop-catalog-service`.

Это разделение позволяет тестировать бизнес-логику отдельно от транспорта.

## Конфиг

Локальный конфиг: [config.local.yaml](/c:/Users/User/Downloads/observability/all_project/shop-gateway/config/config.local.yaml)  
Docker-конфиг: [config.docker.yaml](/c:/Users/User/Downloads/observability/all_project/shop-gateway/config/config.docker.yaml)

Ключевые параметры:

1. `http.port` - HTTP порт gateway.
2. `catalog_grpc.addr` - адрес `catalog-service` (`host:port`).
3. `catalog_grpc.timeout` - таймаут исходящих gRPC вызовов.
4. `otlp.endpoint` - OTLP endpoint (Jaeger/collector).

## Локальный запуск

Из директории `shop-gateway`:

```bash
go run ./cmd/gateway --config config/config.local.yaml
```

Проверка:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
curl http://localhost:8080/metrics
curl "http://localhost:8080/products?limit=5&offset=0"
curl http://localhost:8080/products/prod-001
```

## Docker запуск

### Только gateway контейнер

Из директории `shop-gateway`:

```bash
docker build -t shop-gateway:local .
docker run --rm -p 8080:8080 --name gateway-service shop-gateway:local
```

Важно: для такого запуска `catalog-service` и `jaeger` должны быть доступны по адресам из `config/config.docker.yaml`.

### Через общий compose (рекомендуется)

Из `shop-platform/deploy`:

```bash
docker compose up -d --build jaeger catalog-service gateway-service
```

## API

### `GET /products`

Query:

1. `limit` (optional)
2. `offset` (optional)

Ответ `200`:

```json
{
  "items": [],
  "count": 0
}
```

### `GET /products/{id}`

Ответ `200`:

```json
{
  "id": "prod-001",
  "sku": "SKU-001",
  "name": "Product Name",
  "description": "Description",
  "priceCents": 1999,
  "currency": "USD",
  "stock": 10,
  "active": true
}
```

### Формат ошибок

Все ошибки возвращаются в JSON:

```json
{
  "error": {
    "code": "product_not_found",
    "message": "product not found"
  }
}
```

Стабильные `error.code`:

1. `method_not_allowed`
2. `bad_request`
3. `invalid_pagination`
4. `invalid_product_id`
5. `product_not_found`
6. `internal_error`

## Метрики

Сервисные метрики:

1. `gateway_service_requests_total{method,status}`
2. `gateway_service_request_duration_seconds{method}`

HTTP метрики:

1. `gateway_http_requests_total{method,path,status}`
2. `gateway_http_request_duration_seconds{method,path}`

gRPC client метрики:

1. `gateway_grpc_requests_total{method,code}`
2. `gateway_grpc_request_duration_seconds{method}`

Все доступны через `GET /metrics`.

## Tracing

1. Входящий HTTP запрос инструментирован через `otelhttp`.
2. Service-слой создаёт дочерние spans (`ListProducts`, `GetProduct`).
3. Исходящий gRPC клиент инструментирован через `otelgrpc.NewClientHandler`.
4. Контекст передаётся в `catalog-service`, поэтому trace получается сквозным.

## Структура проекта

```text
shop-gateway/
  cmd/gateway/main.go
  config/
    config.local.yaml
    config.docker.yaml
  internal/
    adapters/catalog_grpc/
    app/
    client/grpc/catalog/
    config/
    domain/
    observability/
    service/gateway/
    transport/http/v1/
  Dockerfile
```

## Проверка готовности перед релизом

1. `go test ./...` проходит.
2. `GET /health` возвращает `200`.
3. `GET /ready` возвращает `200` при доступном `catalog-service`.
4. `GET /metrics` отдаёт `gateway_*` метрики.
5. В Jaeger виден сквозной trace `gateway -> catalog`.
