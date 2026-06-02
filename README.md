# go-payment-gateway

Микросервис платежного шлюза на Go. Проект сделан как backend pet-project: он показывает создание платежей, подтверждение, refund, работу с PostgreSQL, gRPC API и идемпотентные запросы.

## Что умеет сервис

- Создает платеж со статусом `PENDING`.
- Подтверждает платеж и переводит его в `SUCCEEDED`.
- Делает refund и переводит платеж в `REFUNDED`.
- Возвращает платеж вместе с историей транзакций.
- Защищает write-запросы от дублей через `idempotency_key`.
- Хранит платежи, транзакции и idempotency keys в PostgreSQL.

## Стек

- Go
- gRPC / Protocol Buffers
- PostgreSQL
- Docker Compose
- SQL migrations
- pgx

## API

gRPC-контракт описан в файле:

```text
proto/payment/v1/payment.proto
```

Основные методы:

- `CreatePayment` — создание платежа.
- `ConfirmPayment` — подтверждение платежа.
- `RefundPayment` — возврат платежа.
- `GetPayment` — получение платежа по ID.

Для write-операций используется `idempotency_key`. Если повторить тот же запрос с тем же ключом, сервис вернет сохраненный результат. Если использовать тот же ключ с другим телом запроса, сервис вернет ошибку.

## Быстрый запуск

Нужны Go, Git и Docker Desktop.

Запустить весь проект:

```powershell
docker compose up --build -d
```

Проверить контейнеры:

```powershell
docker compose ps
```

Сервис будет доступен по адресу:

```text
localhost:50051
```

Остановить проект:

```powershell
docker compose down
```

Остановить проект и удалить данные PostgreSQL:

```powershell
docker compose down -v
```

## Проверка

Запустить unit-тесты:

```powershell
go test ./...
```

Проверить создание платежа через Docker-версию `grpcurl`:

```powershell
'{"amount":2500,"currency":"EUR","idempotency_key":"demo-create-001"}' |
  docker run --rm -i --network host fullstorydev/grpcurl:latest `
  -plaintext -d '@' localhost:50051 payment.v1.PaymentGateway/CreatePayment
```

Повторный запуск этой же команды должен вернуть тот же `payment.id`. Так проверяется идемпотентность.

## Примеры gRPC-запросов

Создать платеж:

```bash
grpcurl -plaintext \
  -d '{"amount": 1999, "currency": "USD", "idempotency_key": "create-001"}' \
  localhost:50051 payment.v1.PaymentGateway/CreatePayment
```

Подтвердить платеж:

```bash
grpcurl -plaintext \
  -d '{"payment_id": "PASTE_PAYMENT_ID", "idempotency_key": "confirm-001"}' \
  localhost:50051 payment.v1.PaymentGateway/ConfirmPayment
```

Сделать refund:

```bash
grpcurl -plaintext \
  -d '{"payment_id": "PASTE_PAYMENT_ID", "amount": 1999, "idempotency_key": "refund-001"}' \
  localhost:50051 payment.v1.PaymentGateway/RefundPayment
```

Получить платеж:

```bash
grpcurl -plaintext \
  -d '{"payment_id": "PASTE_PAYMENT_ID"}' \
  localhost:50051 payment.v1.PaymentGateway/GetPayment
```

## Генерация gRPC-кода

Go-код для gRPC генерируется из `.proto` файла. На Windows можно использовать готовые скрипты:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\install-tools.ps1
powershell -ExecutionPolicy Bypass -File .\scripts\generate-proto.ps1
```

На macOS/Linux:

```bash
make tools
make proto
```

## Структура проекта

```text
cmd/payment-gateway      точка входа в приложение
proto/payment/v1         gRPC-контракт
gen/payment/v1           сгенерированный protobuf-код
internal/domain          бизнес-сущности и ошибки
internal/usecase         бизнес-логика платежей
internal/repository      работа с PostgreSQL
internal/transport/grpc  gRPC handlers
migrations               SQL-миграции базы данных
```

## Что показывает проект

Проект фокусируется не на количестве кода, а на backend-концепциях:

- проектирование gRPC API;
- работа с PostgreSQL через repository слой;
- атомарные операции через database transactions;
- идемпотентность write-запросов;
- миграции базы данных;
- запуск окружения через Docker Compose;
- разделение кода на `domain`, `usecase`, `repository` и `transport`.
