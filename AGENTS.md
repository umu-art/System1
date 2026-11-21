# AGENTS.md

Что такое System1 — в [README.md](README.md). Здесь — как устроен репозиторий и как в нём работать.

## Структура

```
modules/     код сервисов, по группам
api/         контракты OpenAPI и генерация кода
iac/         образы, манифесты, окружения, ArgoCD
.github/     CI
```

Группа → сервис. Группа — зона ответственности одного человека, каталог в `modules/` без префикса. Сервис — то, что собирается в образ и разворачивается: каталог внутри группы с префиксом `s1-`. Как устроен сервис внутри, решает владелец группы. UI — исключение: это один сервис `s1-ui`, его код лежит прямо в `modules/ui/`.

| Группа | Язык | Владелец | Что в ней | Сервис-шаблон |
|---|---|---|---|---|
| `modules/platform/` | Go | Пётр | API-шлюз, пользователи, диспетчер, знания, диалоги, шлюз каналов, LLM-шлюз | `s1-dialogs` |
| `modules/scenario/` | Go | Александр | движок workflow, агент, валидатор сценариев, продажи, исполнитель задач, коннекторы | `s1-workflow` |
| `modules/copilot/` | Python | Леонид | диалог с автором, сборщик сценариев | `s1-scenario-builder` |
| `modules/analytics/` | Python | Георгий | метрики, анализатор диалогов, детектор аномалий, оценщик качества, инсайты | `s1-metrics` |
| `modules/ui/` | React, TypeScript, Vite | Павел | веб-приложение автора и сотрудника | `s1-ui` (сам каталог) |

Код сервисов не выходит за свою группу. Go-группа — один `go.mod` на все её сервисы, у Python-сервиса свой `requirements.txt`. У каждой группы свой README с запуском.

Весь код, который ходит через контракт, генерируется из `api/src`: Go — серверные интерфейсы, Python — роуты FastAPI и модели, UI — клиент `s1-ui-api`. Как это устроено — в [api/README.md](api/README.md).

## Правила

- Контракты между сервисами — только в `api/src`. Сначала контракт, потом код.
- Сгенерированный код не коммитится. `api/build` в `.gitignore`, собирается командой `python api/build.py`.
- YAML пишется блочно, без фигурных и квадратных скобок.
- Комментарии в коде и YAML — только когда без них непонятно. Пояснения и причины решений выносятся в отдельный файл рядом (`decisions.md`, `README.md`).
- Манифест сервиса в `iac/kubernetes/` не содержит namespace, тега образа и хостов. Отличия окружений — только в `iac/envs/`, патчи stage — в `iac/envs/stage/patches/`.
- Имена: namespace `system1` и `system1-stage`, сервисы и образы `s1-<имя>`.

## Запуск

Нужны Python 3 с `pyyaml` и `build`, Java 17+, Redocly CLI (`npm i -g @redocly/cli`), `wget`, Go 1.25, Node 22.

```bash
python api/build.py

cd modules/platform && go run ./s1-dialogs
cd modules/copilot/s1-scenario-builder && pip install -r requirements.txt && python main.py
cd modules/ui && npm ci && npm run dev
```

Переменные сервисов: `PORT` (по умолчанию 8080), `S1_ENV` (по умолчанию `local`).

## Окружения

| Окружение | Namespace | Откуда берётся |
|---|---|---|
| stage | `system1-stage` | ветка `master`, образы `:master` |
| prod | `system1` | последний git-тег `v*`, образы `:prod` |

Push в master раскатывает stage, тег `v*` раскатывает prod. В ArgoCD один ApplicationSet `system1`.

## Новый сервис

1. Контракт `api/src/<сервис>.yaml` и конфиг генерации по образцу шаблона того же языка: `api/config/go-<сервис>-config.yaml` с `replace` в `go.mod` группы или `api/config/python-<сервис>-config.yaml`. UI-клиент собирается из всех контрактов сам.
2. Каталог `modules/<группа>/<сервис>/` по образцу шаблона группы.
3. `iac/images/<сервис>/Dockerfile`.
4. `iac/kubernetes/<сервис>/` по образцу шаблона того же языка.
5. Строка в матрице `.github/workflows/entrypoint.yml`.
6. Секреты в Vault: `uno/system1-stage/<сервис>` и `uno/system1/<сервис>`.

## Проверка перед коммитом

```bash
python api/build.py
cd modules/<группа> && go vet ./... && go test ./...
cd modules/<группа>/<сервис> && python -m pytest
cd modules/ui && npm run typecheck
```
