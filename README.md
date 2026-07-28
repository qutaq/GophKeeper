# GophKeeper

Менеджер приватных данных (логины/пароли, текст, бинарные файлы, банковские карты, OTP) с синхронизацией между устройствами через gRPC.

## Возможности

- Регистрация и аутентификация пользователей (JWT)
- Хранение приватных данных с E2E-шифрованием
- Синхронизация между устройствами
- CLI-клиент для Windows, Linux, macOS
- Поддержка типов: credentials, text, binary, card, OTP
- Произвольная текстовая метаинформация для любых данных

## Технологии

- **Go** 1.26+
- **gRPC** + **protobuf** — протокол взаимодействия
- **PostgreSQL** — серверное хранилище
- **SQLite** — локальный кэш клиента
- **JWT** — аутентификация и авторизация
- **AES-GCM** — E2E-шифрование данных
- **argon2id** — KDF для master-пароля
- **cobra** — CLI-фреймворк

## Требования

- Go 1.26+
- PostgreSQL 14+
- protoc + плагины (`protoc-gen-go`, `protoc-gen-go-grpc`, `protoc-gen-openapiv2`)
- golangci-lint (опционально, для линтинга)

## Сборка

```bash
# Установить зависимости
go mod tidy

# Собрать сервер и клиент
mkdir -p bin
go build -o bin/gophkeeper-server ./cmd/server
go build -ldflags "-X main.version=dev -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o bin/gophkeeper-client ./cmd/client

# Запустить тесты
go test ./... -cover -race

# Линт
golangci-lint run ./...

# Сгенерировать gRPC-код и OpenAPI из .proto
make generate
# Windows (без Make):
#   powershell -File scripts/generate.ps1
# или вручную:
protoc \
  --proto_path=api/proto \
  --proto_path=third_party/googleapis \
  --proto_path=third_party \
  --go_out=internal/proto --go_opt=paths=source_relative \
  --go_opt=default_api_level=API_OPAQUE \
  --go-grpc_out=internal/proto --go-grpc_opt=paths=source_relative \
  --openapiv2_out=api/swagger \
  --openapiv2_opt=allow_merge=true,merge_file_name=gophkeeper,json_names_for_fields=false \
  api/proto/*.proto
go run ./scripts/swagger2yaml.go -in api/swagger/gophkeeper.swagger.json -out api/swagger.yaml
```

## Кросс-компиляция

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o bin/gophkeeper-client-linux-amd64 ./cmd/client

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o bin/gophkeeper-client-darwin-arm64 ./cmd/client

# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -o bin/gophkeeper-client-darwin-amd64 ./cmd/client

# Windows
GOOS=windows GOARCH=amd64 go build -o bin/gophkeeper-client-windows-amd64.exe ./cmd/client
```

## Информация о версии клиента

```bash
./gophkeeper-client version
```

Выводит версию и дату сборки (передаются через `-ldflags` при компиляции).

## Структура проекта

```
GophKeeper/
├── cmd/
│   ├── server/              # точка входа сервера
│   └── client/              # точка входа CLI-клиента
├── internal/
│   ├── config/              # загрузка конфигов
│   ├── model/               # доменные модели
│   ├── proto/               # сгенерированный gRPC/protobuf-код
│   ├── server/              # логика сервера
│   │   ├── app/             # сборка приложения
│   │   ├── auth/            # регистрация, логин, JWT
│   │   ├── data/            # бизнес-логика CRUD
│   │   ├── sync/            # логика синхронизации
│   │   ├── storage/         # интерфейсы репозиториев
│   │   │   └── postgres/    # реализация на PostgreSQL
│   │   └── grpc/            # gRPC-хендлеры, интерсепторы
│   └── client/              # логика клиента
│       ├── auth/            # обёртки над auth-сервером
│       ├── data/            # бизнес-логика данных
│       ├── storage/         # интерфейс локального хранилища
│       │   └── sqlite/      # реализация на SQLite
│       ├── crypto/          # E2E-шифрование, KDF
│       ├── syncer/          # двусторонняя синхронизация
│       ├── transport/       # gRPC dial + TLS
│       └── cli/             # cobra-команды
├── api/
│   ├── proto/               # .proto файлы (контракт API)
│   ├── swagger/             # промежуточный OpenAPI JSON
│   └── swagger.yaml         # опубликованная OpenAPI-спецификация
├── migrations/              # SQL-миграции (golang-migrate)
├── pkg/                     # переиспользуемые пакеты
├── third_party/             # googleapis + openapiv2 annotations для protoc
├── scripts/                 # вспомогательные скрипты генерации
├── deployments/             # docker-compose, Dockerfile
├── .github/
│   ├── workflows/ci.yml     # lint, test, cross-build
│   └── PULL_REQUEST_TEMPLATE.md
├── Makefile
├── go.mod
└── go.sum
```

## Протокол (фаза 1)

Сервисы gRPC (`gophkeeper.v1`):

| Сервис | RPC |
|--------|-----|
| `AuthService` | `Register`, `Login`, `Refresh`, `Logout` |
| `DataService` | `AddItem`, `UpdateItem`, `DeleteItem`, `ListItems`, `GetItem` |
| `SyncService` | `Sync` |

Типы данных: `CREDENTIALS`, `TEXT`, `BINARY`, `CARD`, `OTP`. Полезная нагрузка `Item.encrypted_data` приходит уже зашифрованной клиентом; метаинформация — `map<string,string>`.

## Хранилище (фаза 2)

- Миграции: `migrations/` (golang-migrate)
- Репозитории: `internal/server/storage` + PostgreSQL/`pgx` в `internal/server/storage/postgres`
- At-rest шифрование: `pkg/crypto/atrest` (AES-256-GCM; `KeyProvider` — шов под KMS)
- Индекс синхронизации: `items(owner_id, updated_at)` (+ `owner_id, version`)

```bash
# Поднять PostgreSQL
docker compose -f deployments/docker-compose.yml up -d

# Применить миграции (из кода сервера или вручную)
# DSN: postgres://gophkeeper:gophkeeper@localhost:5432/gophkeeper?sslmode=disable
# migrate URL: pgx5://gophkeeper:gophkeeper@localhost:5432/gophkeeper?sslmode=disable

# Интеграционный тест репозиториев
DATABASE_URL='postgres://gophkeeper:gophkeeper@localhost:5432/gophkeeper?sslmode=disable' \
  go test ./internal/server/storage/postgres/ -count=1
```

## Аутентификация (фаза 3)

- Пароли: argon2id (`internal/server/auth`)
- Access JWT (HS256) + opaque refresh token (SHA-256 hash в БД)
- Ротация ключей подписи: `GOPHKEEPER_JWT_KEYS` + `GOPHKEEPER_JWT_CURRENT_KID` (`kid` в JWT header)
- gRPC interceptor: `Authorization: Bearer <access>`; публичные методы — Register/Login/Refresh/Logout
- Пример env: [`.env.example`](.env.example)

```bash
# Запуск сервера (после docker compose up)
export DATABASE_URL='postgres://gophkeeper:gophkeeper@localhost:5432/gophkeeper?sslmode=disable'
export GOPHKEEPER_ATREST_KEY='0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef'
export GOPHKEEPER_JWT_KEYS='v1:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef'
go run ./cmd/server
```

## Данные и синхронизация (фаза 4)

- `DataService`: Add/Update/Delete/List/Get — только для `owner_id` из JWT
- `SyncService.Sync`: изменения с `since_version` / `since_timestamp`, включая soft-delete
- Конфликты Update: при `expected_version > 0` — optimistic concurrency (mismatch → `Aborted`, сервер wins + лог); при `expected_version == 0` — last-write-wins overwrite + лог

## Клиент CLI (фаза 5)

Команды:

```bash
gophkeeper-client version
gophkeeper-client register --login alice
gophkeeper-client login --login alice
gophkeeper-client add credentials --meta site=example.com
gophkeeper-client add text --meta title=note
gophkeeper-client list
gophkeeper-client get <id>
gophkeeper-client edit <id>
gophkeeper-client delete <id>
gophkeeper-client sync
```

- Конфиг: `~/.gophkeeper/config.yaml` или флаги/`GOPHKEEPER_*`
- Локальный vault: SQLite `~/.gophkeeper/vault.db`, ключ = argon2id(master password, salt=SHA256(login))
- Данные шифруются на клиенте до записи локально и до отправки на сервер
- `sync`: pull (server SoT по version) + push dirty

## Безопасность (фаза 6)

- **TLS**: обязателен на сервере (`GOPHKEEPER_TLS_CERT` / `GOPHKEEPER_TLS_KEY`) и клиенте (`ca_file`, опционально mTLS `cert_file`/`key_file`); plaintext не поддерживается — в dev используйте самоподписанные сертификаты
- **E2E**: полезные нагрузки шифруются AES-256-GCM master-ключом (argon2id из master-пароля) до локального хранения и синхронизации
- **Сервер**: хранит только шифротекст клиента + дополнительный at-rest слой (`GOPHKEEPER_ATREST_KEY`)
- **Master-пароль**: salt детерминирован от login (кросс-девайс), verifier в локальном vault проверяет корректность ключа
- **Секреты в памяти**: master-ключ в `pkg/secure` (best-effort mlock/VirtualLock + Zero); JWT access/refresh в SQLite только в зашифрованном виде

```bash
# Сервер с TLS (обязательно)
export GOPHKEEPER_TLS_CERT=/path/to/server.crt
export GOPHKEEPER_TLS_KEY=/path/to/server.key
go run ./cmd/server

# Клиент
# ~/.gophkeeper/config.yaml:
#   tls:
#     ca_file: /path/to/ca.crt
```

## Тесты и покрытие (фаза 7)

```bash
# Все тесты
go test ./... -cover -race

# Порог покрытия ≥70% (без generated proto / CLI / cmd / helpers)
make cover
# или:
bash scripts/check-coverage.sh

# Интеграция PostgreSQL (репозитории)
docker compose -f deployments/docker-compose.yml up -d
DATABASE_URL='postgres://gophkeeper:gophkeeper@localhost:5432/gophkeeper?sslmode=disable' \
  go test ./internal/server/storage/postgres/ -count=1
```

Что покрыто:
- unit: server `auth` / `data` / `sync`, client `crypto` / `sqlite` / `data` / `auth` / `syncer` / `transport`, `config`
- integration: in-memory gRPC (`bufconn`) — Register → Login → CRUD → Sync; чужие записи недоступны
- CI: Postgres service + coverage gate + artifact `coverage.out`

Клиентский конфиг-пример: [`configs/client.example.yaml`](configs/client.example.yaml).

## Запуск сервера

```bash
cp .env.example .env   # при необходимости
docker compose -f deployments/docker-compose.yml up -d
go run ./cmd/server
```

Клиент:

```bash
go run ./cmd/client version
go run ./cmd/client --server localhost:50051 register --login alice
```

## Документация

- OpenAPI-спецификация: [`api/swagger.yaml`](api/swagger.yaml) (генерируется из `.proto`)
