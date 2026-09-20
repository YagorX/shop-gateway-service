# internal/client/grpc

Транспортные gRPC-клиенты. Знают protobuf, TLS и таймауты; не знают ничего про домен и HTTP.

```text
client/grpc/
├── auth/     # AuthService: 6 методов
├── catalog/  # CatalogService: unary + server-streaming
└── cart/     # CartService: 5 методов
```

## Создание соединения

Все три клиента устроены одинаково:

```go
conn, err := grpc.DialContext(
    dialCtx,                                          // с таймаутом из конфига
    addr,
    grpc.WithTransportCredentials(transportCreds),    // mTLS или insecure
    grpc.WithBlock(),                                 // ждать готовности
    grpc.WithStatsHandler(otelgrpc.NewClientHandler()), // трейсинг
)
```

`WithStatsHandler` даёт клиентские спаны автоматически: каждый исходящий вызов становится дочерним span к текущему, а `traceparent` уезжает в метаданных запроса. Именно это делает видимыми сквозные цепочки в Jaeger.

## mTLS

`buildTransportCredentials(tlsCfg)` собирает `tls.Config`:

| Поле | Источник | Роль |
|---|---|---|
| `RootCAs` | `ca_file` | Проверка сертификата сервера |
| `Certificates` | `client_cert_file` + `client_key_file` | Предъявление себя серверу |
| `ServerName` | `server_name` | Имя, которое должно быть в SAN сертификата сервера |
| `MinVersion` | константа | TLS 1.2 |

Двусторонняя проверка: клиент убеждается, что сервер подписан доверенным CA и его имя совпадает с ожидаемым, сервер — что клиент предъявил сертификат от того же CA.

`ServerName` задаётся явно, потому что адрес подключения и ожидаемое имя могут расходиться. Показательный случай — каталог: gateway подключается к `proxy-service:9095`, TLS-сессия терминируется на прокси, поэтому `catalog_tls.server_name: proxy-service`, хотя запрос в итоге обслужит `catalog-service`.

При `enabled: false` возвращается `insecure.NewCredentials()` — режим локальной разработки без сертификатов.

## Инструментирование вызовов

Каждый метод обёрнут одинаково:

```go
startedAt := time.Now()
grpcCode := codes.OK.String()
defer func() {
    metrics.GatewayGRPCRequestDuration.WithLabelValues("AuthRegister").Observe(time.Since(startedAt).Seconds())
    metrics.GatewayGRPCRequestsTotal.WithLabelValues("AuthRegister", grpcCode).Inc()
}()

reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
defer cancel()
```

Метка `code` заполняется из `status.Code(err)` перед возвратом, поэтому в метриках видно распределение по кодам ответа, а не только успех/ошибка.

Таймаут на каждый вызов — своя защита от повисших запросов: даже если контекст запроса пришёл без дедлайна, вызов не будет ждать вечно.

## Стриминг в catalog

`StreamProducts` возвращает обёртку над `grpc.ClientStream`, реализующую интерфейс `ProductStream` сервисного слоя. Элементы читаются по одному через `Recv()`; `io.EOF` означает штатное завершение потока.

Контекст стрима **не должен** отменяться сразу после создания — отмена закрывает поток. Поэтому `cancel` здесь передаётся вызывающему вместе со стримом, а не вызывается через `defer`.

## Известные проблемы

* **`grpc.DialContext` и `WithBlock()` помечены deprecated.** Актуальный способ — `grpc.NewClient`, который подключается лениво. Текущий вариант блокирует старт сервиса до готовности зависимости: это первопричина crash-loop контейнера gateway, когда `catalog-service` ещё поднимается.
* **Нет service config.** Не заданы политика ретраев (`grpc.WithDefaultServiceConfig` с `retryPolicy`), keepalive-параметры и балансировка. Для `cart-service`, работающего с Mongo replica set, retry на `UNAVAILABLE` был бы уместен.
* **Метки метрик — строковые литералы** (`"AuthRegister"`), продублированные в каждом методе рядом с константой `op`.
* **Повтор `buildTransportCredentials`.** Функция реализована трижды — в auth-, catalog- и cart-клиенте, плюс четвёртая копия в readiness-чекерах (`buildAuthTransportCredentials` в [`transport/grpc/v1/handlers`](../../transport/grpc/v1/handlers)). Один общий хелпер убрал бы риск, что исправление TLS-настроек внесут не во все копии.
* **Сертификаты читаются с диска один раз при старте.** Обновление сертификата требует перезапуска процесса; `tls.Config.GetClientCertificate` позволил бы перечитывать без рестарта.
