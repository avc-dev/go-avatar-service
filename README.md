# GophProfile — сервис аватарок

Маленький микросервис на Go, который хранит аватарки пользователей и отдаёт
их по REST API. Пользователь загружает картинку один раз, дальше любой
внешний сервис (форум, блог, виджет комментариев) может получить аватар по
ID пользователя.

Оригинал лежит в S3-совместимом хранилище (локально — MinIO), миниатюры
100×100 и 300×300 генерирует отдельный воркер асинхронно через очередь.

## Что внутри

- HTTP-сервер на `chi`
- Postgres через `pgx/v5`, миграции на `golang-migrate`
- MinIO через `minio-go/v7`
- RabbitMQ через `amqp091-go` (topic-обменник + DLX для terminal-фейлов)
- Ресайз картинок — `disintegration/imaging` (чистый Go, без CGO)
- Структурные логи `log/slog`
- Конфиг из env (`caarlos0/env/v11`)
- Тесты — `testify` + `testcontainers-go`, моки через `mockery`

## Как запустить — всё в Docker

Нужен Docker Desktop (или engine + compose v2).

```bash
make up        # поднимет postgres, minio, rabbitmq, migrator, app-server, app-worker
make ps        # проверить что все контейнеры healthy
make logs      # хвост логов compose
```

После старта:

- `http://localhost:8080/web/` — веб-интерфейс (загружай/смотри/удаляй аватарки)
- `http://localhost:8080/health` — статус компонентов (200 если всё ок, 503 если что-то отвалилось)
- `http://localhost:15673/` — RabbitMQ Management UI (guest/guest)
- `http://localhost:9001/` — MinIO Console (minioadmin/minioadmin)

> ⚠️ Пароли в `compose.yaml` и `.env.example` — это **dev-дефолты**, чтобы
> можно было собрать стек одной командой. В реальном деплое их нужно убрать
> в secret-store и заменить переменными окружения.

```bash
make down      # остановить, volume'ы не трогаем
```

## Как запустить — приложение на хосте, зависимости в Docker

Так обычно удобнее работать: правишь Go-код, видишь рестарт без пересборки
образа.

```bash
make up                  # только инфраструктура (если ещё не поднята)
cp .env.example .env     # уже настроен на локальные порты
make migrate-up          # накатить схему
make run-server          # терминал A
make run-worker          # терминал B
```

`.env` смотрит на **нестандартные host-порты**, чтобы не конфликтовать с
другими проектами на той же машине:

- Postgres — `5433` (а не 5432)
- RabbitMQ AMQP — `5673`, Management UI — `15673`
- MinIO — стандартные 9000/9001 (там обычно никто не сидит)

Внутри compose сервисы между собой общаются на стандартных портах через
service-name (`postgres:5432`, `rabbitmq:5672` и т.д.) — это переопределено
в `compose.yaml` через `environment:` отдельно от `.env`.

## API

| Метод | Путь | Auth | Что делает |
|---|---|---|---|
| `POST` | `/api/v1/avatars` | `X-User-ID` | Загрузить файл (multipart, поле `file`, до 10 MB) |
| `GET` | `/api/v1/avatars/{id}` | — | Стрим оригинала |
| `GET` | `/api/v1/avatars/{id}?size=100x100` | — | Стрим миниатюры (доступные размеры: `100x100`, `300x300`) |
| `GET` | `/api/v1/avatars/{id}/metadata` | — | JSON с метаданными и URL'ами миниатюр |
| `DELETE` | `/api/v1/avatars/{id}` | `X-User-ID`, владелец | Soft-delete + асинхронная очистка S3 |
| `GET` | `/api/v1/users/{user_id}/avatar` | — | Стрим текущей (последней) аватарки пользователя |
| `GET` | `/api/v1/users/{user_id}/avatars` | — | Список всех неудалённых аватарок пользователя |
| `DELETE` | `/api/v1/users/{user_id}/avatar` | `X-User-ID == user_id` | Удалить текущую аватарку |
| `GET` | `/health` | — | Статус Postgres/MinIO/RabbitMQ |
| `GET` | `/web/*` | — | Статика SPA |

`X-User-ID` принимается **только в формате UUID v7**. Middleware режет
любой другой UUID c 400. Та же проверка применяется к `{user_id}` в пути.

Чтения публичны — это нужный кейс «внешний сайт показывает аватар»: ему
авторизация не нужна, нужен только идентификатор пользователя.

## Структура

```
cmd/
  server/      HTTP-сервер
  worker/      обработчик очереди
internal/
  broker/      Publisher + Consumer для RabbitMQ (плюс NoOp для тестов)
  config/      env-конфиг
  domain/      Avatar и доменные события
  handlers/    HTTP-хендлеры + health
  httpx/       общие хелперы записи JSON-ответов
  imageproc/   Decode → Fill → JPEG
  logger/      slog setup
  middleware/  RequestID-friendly logger и валидатор X-User-ID
  repository/  pgx-обвязка
  services/    use-case оркестрация (сага компенсации в Upload)
  storage/     MinIO-адаптер
  worker/      обработчики событий (идемпотентны через DB-статус)
migrations/    SQL-миграции
tests/         e2e-тест со всеми зависимостями (testcontainers, build tag `integration`)
web/static/    SPA (один HTML, Tailwind через CDN, vanilla JS)
```

Внутри каждого слоя — поддиректория по агрегату (`internal/services/avatar/`,
`internal/handlers/avatar/`), и **по файлу на публичный метод**. Имена типов
без префикса агрегата: `avatar.Service`, `avatar.Repository`, не
`AvatarService` / `AvatarRepository`.

## Тесты

```bash
make test              # юнит + пер-пакетные интеграционные (testcontainers внутри)
make test-integration  # e2e через весь стек (testcontainers, ~15-30s)
make lint              # golangci-lint
```

E2e-тест поднимает реальные PG/MinIO/RMQ в testcontainers, запускает
in-process воркер и HTTP-сервер, делает полный цикл upload → ждёт миниатюр
→ скачивает → удаляет → проверяет что worker почистил S3.

## Make-таргеты

```
make up                  поднять весь стек
make down                остановить
make logs / make ps      что там в compose
make run-server          запустить сервер на хосте против дockerized-зависимостей
make run-worker          то же для воркера
make migrate-up/down     накатить/откатить миграции (через host migrate CLI)
make test                юнит-тесты
make test-integration    e2e
make lint                golangci-lint
make mocks               перегенерить моки через mockery
make build               собрать бинарники в bin/
make docker-build        собрать app-image без запуска
make help                этот список
```

## Что нужно поставить на хост для dev

- Go 1.25+
- Docker (Desktop или engine + compose v2)
- `migrate` CLI — если хочешь использовать `make migrate-up` с хоста:
  ```
  go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
  ```
- `mockery` v2 — если будешь править интерфейсы и регенерить моки:
  ```
  go install github.com/vektra/mockery/v2@latest
  ```
- `golangci-lint` v2:
  ```
  brew install golangci-lint
  # или
  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
  ```

В compose ничего из этого не требуется — migrator там отдельный one-shot
сервис на официальном образе `migrate/migrate`.

## Чего пока нет

Прямо сейчас сервис работает, но это всё ещё MVP. Что в голове на потом:

- Автореконнект к RabbitMQ — сейчас если коннект порвался, процесс падает.
  Перезапуск процесса обходит проблему, но в проде хочется аккуратнее.
- Outbox-паттерн или reconciliation-job для случая «положили в БД, не
  успели опубликовать событие». Сейчас в этом случае пишем WARN и идём
  дальше, аватар остаётся со статусом `pending`.
- Если воркер упал ровно посреди обработки, запись зависает в статусе
  `processing`. Нужна джоба-сторож, которая возвращает их в `pending` через
  N минут.
- Валидация MIME через magic bytes, rate limit, CORS — не сделано, ждёт
  своей очереди.
- `width`/`height` в metadata — в схеме нет; добавим, если появится
  потребность.
- CI пока не настроен.

## Спецификация

Исходное ТЗ лежит в `SPECIFICATION.md` (не публикуется — это материал курса).
