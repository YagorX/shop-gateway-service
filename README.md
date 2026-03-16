# shop-gateway

`shop-gateway` — внешний API gateway проекта mini-shop.

Сервис принимает HTTP(S)-запросы, отдает operational endpoints, проксирует product-запросы в `shop-catalog-service` по gRPC и auth-запросы в `shop-auth` по gRPC mTLS.

## Что умеет сервис

1. Отдавать product API:
   - `GET /products`
   - `GET /products/{id}`
2. Отдавать auth API:
   - `POST /auth/register`
   - `POST /auth/login`
   - `POST /auth/validate`
   - `POST /auth/refresh`
   - `POST /auth/logout`
   - `POST /auth/is-admin`
3. Отдавать operational endpoints:
   - `GET /health`
   - `GET /ready`
   - `GET /metrics`
   - `GET/POST /admin/log-level`
4. Проксировать внутренние вызовы в:
   - `catalog-service` по gRPC
   - `auth-service` по gRPC mTLS

## Security model

1. В Docker-окружении gateway публикуется наружу по HTTPS.
2. Канал `gateway -> auth-service` защищен клиентским и серверным сертификатами через mTLS.
3. Gateway работает как edge-компонент для JWT-based auth flow.
4. Сам gateway не хранит пользовательские пароли и не генерирует токены, а делегирует это `shop-auth`.

## Архитектура

Слои:

1. `transport/http/v1` — handlers, router, JSON contract, health/admin endpoints.
2. `service/gateway` — orchestration, logging, metrics, tracing.
3. `adapters/catalog_grpc` и `adapters/auth_grpc` — реализация портов.
4. `client/grpc/catalog` и `client/grpc/auth` — транспортный слой gRPC клиентов.
5. `app/*` — bootstrap и lifecycle.

Поток product-запроса:

1. HTTP запрос приходит в products handler.
2. Handler вызывает `GatewayService`.
3. Service пишет метрики и spans.
4. Catalog adapter вызывает gRPC client.
5. Вызов уходит в `shop-catalog-service`.

Поток auth-запроса:

1. HTTP запрос приходит в auth handler.
2. Handler вызывает `GatewayService`.
3. Service пишет метрики и spans.
4. Auth adapter вызывает gRPC client с TLS credentials.
5. Вызов уходит в `shop-auth`.

## Конфигурация

Файлы:

1. `config/config.local.yaml`
2. `config/config.docker.yaml`

Ключевые поля:

1. `http.port` — порт gateway (`8083`)
2. `http_tls.*` — внешний HTTPS сервер
3. `catalog_grpc.addr` — адрес `catalog-service`
4. `catalog_grpc.timeout` — таймаут каталожных gRPC вызовов
5. `auth_grpc.addr` — адрес `auth-service`
6. `auth_grpc.timeout` — таймаут auth gRPC вызовов
7. `auth_tls.*` — CA, `server_name` и client cert/key для mTLS
8. `otlp.endpoint` — OTLP endpoint для traces

Локальная конфигурация по умолчанию:

1. `http_tls.enabled: false`
2. `auth_tls.enabled: false`

Docker-конфигурация:

1. `http_tls.enabled: true`
2. `auth_tls.enabled: true`

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
docker compose up -d --build auth-service catalog-service gateway-service
```

## Проверка Docker-режима

В Docker окружении gateway ожидает HTTPS, поэтому для host-side CLI нужен `-k`:

```bash
curl -k https://localhost:8083/health
curl -k https://localhost:8083/ready
curl -k https://localhost:8083/metrics
curl -k https://localhost:8083/products
```

Пример register:

```bash
curl -k -X POST https://localhost:8083/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"demo","email":"demo@example.com","password":"Test123!"}'
```

Пример login:

```bash
curl -k -X POST https://localhost:8083/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email_or_name":"demo","password":"Test123!","app_id":1,"device_id":"dev-1"}'
```

## API

### Products

`GET /products`

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

`GET /products/{id}`

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

### Auth

`POST /auth/register`

```json
{
  "username": "demo",
  "email": "demo@example.com",
  "password": "Test123!"
}
```

`POST /auth/login`

```json
{
  "email_or_name": "demo",
  "password": "Test123!",
  "app_id": 1,
  "device_id": "dev-1"
}
```

`POST /auth/validate`

```json
{
  "token": "<access_token>",
  "app_id": 1
}
```

### Формат ошибок

Все ошибки отдаются в JSON:

```json
{
  "error": {
    "code": "internal_error",
    "message": "internal error"
  }
}
```

Стабильные `error.code`:

1. `method_not_allowed`
2. `bad_request`
3. `invalid_pagination`
4. `invalid_product_id`
5. `product_not_found`
6. `already_exists`
7. `not_found`
8. `unauthenticated`
9. `internal_error`

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

1. Входящий HTTP инструментирован через `otelhttp`.
2. Service-слой создает spans вида `service.gateway.*`.
3. Исходящие gRPC клиенты инструментированы через `otelgrpc.NewClientHandler`.
4. В Jaeger видны цепочки:
   - `gateway -> catalog`
   - `gateway -> auth`

## Readiness

`/ready` для gateway проверяет две зависимости:

1. `catalog-service` через gRPC health-check
2. `auth-service` через gRPC health-check c TLS credentials

Если хотя бы одна зависимость недоступна, gateway отвечает `503`.

## Структура проекта

```text
shop-gateway/
  cmd/
    gateway/main.go
  config/
    config.local.yaml
    config.docker.yaml
  internal/
    adapters/
      auth_grpc/
      catalog_grpc/
    app/
    client/grpc/
      auth/
      catalog/
    config/
    domain/
    observability/
    service/gateway/
    transport/http/v1/
  Dockerfile
```

## Чеклист готовности

1. `GET /health` возвращает `200`
2. `GET /ready` возвращает `200`, когда доступны `catalog-service` и `auth-service`
3. `GET /metrics` отдает `gateway_*`
4. `GET /products` и auth flow работают через gateway
5. В Jaeger видны trace chains для auth и catalog
6. В Docker-режиме gateway отвечает по HTTPS
7. Канал `gateway -> auth-service` защищен mTLS
