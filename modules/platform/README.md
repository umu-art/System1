# platform

Технический скелет ядра: API-шлюз, пользователи, диспетчер, знания, диалоги, шлюз каналов, LLM-шлюз. Владелец — Пётр. Go.

Сервисы: `s1-dialogs` (шаблон группы) и `s1-channels-gateway`. Один `go.mod` на все сервисы группы, код из контрактов подключается через `replace`, код store генерирует sqlc. Как устроен сервис — в [rules.md](rules.md).

## Запуск

Нужны Go 1.25, Docker и всё, что требует `api/build.py`.

```bash
make gen
make seed
make run
curl localhost:8080/readyz
curl localhost:8081/readyz
```

`make gen` генерирует код из контрактов и sqlc, его нужно запускать после изменения контрактов, миграций или запросов в `store/queries/`.

`make run` поднимает Postgres, применяет миграции и запускает все сервисы. Один сервис запускается через `make run-dialogs` или `make run-gateway`.

## Проверки

```bash
make check
```

`make check` запускает `vet`, `lint` и все тесты. Интеграционным тестам нужен Docker, Postgres они поднимают сами.

## Переменные

| Переменная | По умолчанию | Что это |
|---|---|---|
| `PORT` | `8080` | Порт HTTP-сервера |
| `S1_ENV` | `local` | Окружение |
| `LOG_LEVEL` | `info` | Уровень логов: `debug`, `info`, `warn`, `error` |
| `DATABASE_URL` | — | Строка подключения к Postgres, обязательная |

Локальные значения лежат в `local.env`, личные — в `.env`.
