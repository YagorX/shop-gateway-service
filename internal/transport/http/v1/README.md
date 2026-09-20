# internal/transport/http/v1

HTTP-слой gateway: маршрутизация, разбор запросов, сериализация ответов, единый формат ошибок. Бизнес-логики нет — хендлеры вызывают сервисный слой через интерфейсы из [`contracts`](contracts).

```text
http/v1/
├── router.go       # сборка ServeMux и цепочки middleware
├── router_test.go
├── contracts/      # ProductService, AuthService, CartService,
│                   # LogLevelController, ReadinessChecker
├── handlers/
│   ├── products.go # список, по id, SSE-поток
│   ├── cart.go     # пять операций с корзиной
│   ├── auth.go     # шесть auth-эндпоинтов
│   ├── health.go   # /health, /ready
│   ├── admin.go    # /admin/log-level
│   ├── status.go   # HTML-страница через html/template
│   └── swagger.go  # Swagger UI, ReDoc, отдача спеки
└── middleware/     # → middleware/README.md
```

## Роутер

Используется стандартный `http.ServeMux` без внешних роутеров. Защищённые маршруты оборачиваются точечно:

```go
mux.Handle("/products", authMiddleware(http.HandlerFunc(productsHandler.List)))
mux.HandleFunc("/auth/login", authHandler.Login)  // без auth
```

`/products/` регистрируется с завершающим слэшем — в `ServeMux` это префиксный шаблон, он ловит `/products/{id}`, а идентификатор хендлер достаёт из пути сам.

`RouterDeps` собирает все зависимости в одну структуру. Поля-интерфейсы означают, что роутер можно собрать с тестовыми реализациями — этим пользуется [`router_test.go`](router_test.go).

## Контракты

[`contracts/ports.go`](contracts/ports.go) описывает нужное транспорту. Один объект `GatewayService` реализует сразу `ProductService`, `AuthService` и `CartService`, поэтому в `RouterDeps` он передаётся трижды — но транспорт об этом не знает и знать не должен.

## Формат ответов

Успех — JSON тела ресурса. Ошибка — всегда одинаковая структура:

```json
{ "error": { "code": "product_not_found", "message": "product not found" } }
```

`code` — стабильный машинночитаемый идентификатор, на который может опираться клиент; `message` — человекочитаемое пояснение. Внутренние тексты ошибок наружу не уходят.

## SSE-поток

`/products/stream` — самый нестандартный хендлер. Схема:

```go
flusher, ok := w.(http.Flusher)      // без Flusher стриминг невозможен
w.Header().Set("Content-Type", "text/event-stream")
// для каждого элемента:
fmt.Fprintf(w, "event: product\ndata: %s\n\n", payload)
flusher.Flush()
```

Три типа событий:

| Событие | Когда |
|---|---|
| `event: product` | Очередной товар, `data` — JSON объекта |
| `event: end` | Поток дошёл до `io.EOF`, `data: {"status":"completed"}` |
| `event: error` | Ошибка чтения из gRPC-стрима или сериализации |

`Flush()` после каждого события обязателен: без него Go буферизует ответ и клиент получит всё разом в конце, что лишает стриминг смысла.

Ошибка передаётся **внутри потока**, а не HTTP-статусом — заголовки уже отправлены со `200`, изменить их нельзя. Это общее свойство любого стриминга, и клиент должен обрабатывать `event: error`.

## Health и ready

`/health` отвечает `200`, пока процесс жив. `/ready` опрашивает `MultiReadinessChecker` — все три gRPC-зависимости (см. [`transport/grpc/v1/handlers`](../../grpc/v1/handlers)); недоступность любой даёт `503`.

## Статусная страница

`/status` рендерится через `html/template` из файла по пути `template_path`. Показывает имя сервиса, окружение, версию, адрес, включён ли HTTPS и время генерации. `html/template` (а не `text/template`) экранирует значения по контексту — подстановка из конфига не приведёт к XSS.

Маршрут регистрируется только если шаблон указан в конфиге, иначе его просто нет.

## Swagger

| Путь | Содержимое |
|---|---|
| `/swagger/` | Swagger UI из `web/swagger/index.html` |
| `/redoc/` | ReDoc из `web/redoc/index.html` |
| `/swagger/spec/gateway.v1.yaml` | Спека через `http.FileServer` |

Спека — сборка из многофайлового источника в [`shop-contracts`](../../../../../shop-contracts): `make bundle` инлайнит все `$ref` и кладёт результат в `openapi/gateway.v1.yaml` этого репозитория. Swagger UI не умеет ходить по относительным `$ref` между файлами, поэтому нужен именно бандл.

## Известные проблемы

* **Неверные метки путей в метриках корзины.** `RemoveItem`, `UpdateItem` и `ClearCart` в [`handlers/cart.go`](handlers/cart.go) пишут `path="/cart/items"`, хотя обслуживают `/cart/items/remove`, `/cart/items/update` и `/cart/clear`. В Prometheus три разных эндпоинта сливаются в один.
* **`WriteTimeout: 15s` обрывает длинные SSE-потоки.** Таймаут задан в [`internal/app`](../../../app) и применяется ко всему ответу целиком. Для стримингового маршрута нужен отдельный сервер или `http.ResponseController.SetWriteDeadline`.
* **RPC-стиль маршрутов.** `/cart/items/update`, `/cart/items/remove`, `/cart/clear` не согласуются с остальным REST-API; ожидались бы `PATCH /cart/items/{id}`, `DELETE /cart/items/{id}`, `DELETE /cart`.
* **`/admin/log-level` и `/status` не защищены**, хотя порт публичный.
* **Дублирование `writeError` / `writeJSON`** — свои копии есть и в `handlers`, и в `middleware`.
* **`ServeMux` без явной проверки метода на части маршрутов.** Go 1.22+ поддерживает шаблоны вида `"POST /cart/items"`, что убрало бы ручные проверки `r.Method` из хендлеров. Сейчас стиль смешанный: часть проверяет метод внутри, часть полагается на mux.
