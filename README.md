# shop-gateway

`shop-gateway` — HTTP API gateway для доступа к `shop-catalog-service` по gRPC.

Сервис принимает HTTP-запросы, выполняет бизнес-валидацию в service-слое и проксирует чтение каталога в `catalog-service`.

## Возможности

1. HTTP endpoints:
   - `GET /products`
   - `GET /products/{id}`
   - `GET /health`
   - `GET /ready`
   - `GET /metrics`
   - `GET/POST /admin/log-level`
2. Слоистая архитектура:
   - `transport/http` (handlers + router)
   - `service/gateway` (бизнес-логика)
   - `adapters/catalog_grpc` (адаптер порта)
   - `client/grpc/catalog` (gRPC транспорт)
3. Observability:
   - JSON-логи (`slog`)
   - Prometheus-метрики (`gateway_*`)
   - OpenTelemetry-трейсинг (HTTP + gRPC client)

## Архитектура запроса

1. HTTP-запрос приходит в `handlers/products.go`.
2. Handler вызывает `GatewayService`.
3. `GatewayService` валидирует вход и вызывает интерфейс `ProductRepository`.
4. `CatalogAdapter` реализует `ProductRepository` через `client/grpc/catalog`.
5. gRPC вызов уходит в `shop-catalog-service`.

## Конфигурация

Файлы:

1. `config/config.local.yaml`
2. `config/config.docker.yaml`

Ключевые поля:

1. `http.port` — порт HTTP сервера (`8083`)
2. `catalog_grpc.addr` — адрес `catalog-service` (`host:port`)
3. `catalog_grpc.timeout` — таймаут исходящих gRPC-вызовов
4. `otlp.endpoint` — OTLP endpoint для трейсов

## Локальный запуск

Из директории `shop-gateway`:

```bash
go run ./cmd/gateway --config config/config.local.yaml
```

Проверка:

```bash
curl http://127.0.0.1:8083/health
curl http://127.0.0.1:8083/ready
curl http://127.0.0.1:8083/metrics
curl "http://127.0.0.1:8083/products?limit=5&offset=0"
curl http://127.0.0.1:8083/products/prod-001
```

## Docker запуск

### Только gateway контейнер

```bash
docker build -t shop-gateway:local .
docker run --rm -p 8083:8083 --name gateway-service shop-gateway:local
```

### Через общий compose

Из `shop-platform/deploy`:

```bash
docker compose up -d --build jaeger postgres redis catalog-service gateway-service
```

## API

### `GET /products`

Query:

1. `limit` (optional)
2. `offset` (optional)

Успех:

```json
{
  "items": [],
  "count": 0
}
```

### `GET /products/{id}`

Успех:

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

Все ошибки отдаются в JSON:

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

Service:

1. `gateway_service_requests_total{method,status}`
2. `gateway_service_request_duration_seconds{method}`

HTTP:

1. `gateway_http_requests_total{method,path,status}`
2. `gateway_http_request_duration_seconds{method,path}`

gRPC client:

1. `gateway_grpc_requests_total{method,code}`
2. `gateway_grpc_request_duration_seconds{method}`

## Tracing

1. Входящий HTTP-трафик инструментирован через `otelhttp`.
2. Service-слой создает child spans (`service.gateway.ListProducts`, `service.gateway.GetProduct`).
3. Исходящий gRPC клиент инструментирован через `otelgrpc.NewClientHandler`.
4. Контекст прокидывается в `catalog-service`, trace сквозной.

## Readiness

`/ready` проверяет доступность `catalog-service` через `grpc.health.v1.Health/Check`.

Для стабильного readiness-check в gateway используется блокирующее подключение (`DialContext + WithBlock`) и ограниченный timeout.

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

## Чеклист готовности

1. `go test ./...` проходит
2. `GET /health` возвращает `200`
3. `GET /ready` возвращает `200` при доступном `catalog-service`
4. `GET /metrics` отдает `gateway_*`
5. В Jaeger виден trace `gateway -> catalog`
