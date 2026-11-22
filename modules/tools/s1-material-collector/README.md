# S1 Material Collector

Go CronJob выгружает текстовые расшифровки двух комнат МТС Линк в папку `System1/MTS Link Export`. Внутри неё лежат каталоги `Баня`, `System1` и `state.json`. Коллектор подбирает расшифровки с `createdAt` от 11 ноября 2026 года, 00:00 МСК, включительно. Последующие запуски пропускают уже выгруженные файлы. Коллектор работает в prod раз в час.

```sh
cd modules/tools
go vet ./...
go test ./...
go build -o bin/s1-material-collector ./s1-material-collector
```

## Подключение

Для команд нужен kubeconfig целевого кластера и право записать два Secret в `system1`. Перед инициализацией проверьте шифрование Secrets в действующей конфигурации kube-apiserver.

Секрет МТС Линк создаётся из локального файла HTTP Client с идентификаторами двух комнат и заголовками `Set-Cookie` из свежего ответа `BrowserRefresh`. Храните файл `a.http` локально. Команда подходит и для ручной замены cookies.

```sh
./bin/s1-material-collector configure-mts \
  --context '<prod-context>' \
  --http-file s1-material-collector/a.http
```

Для Google Drive включите **Google Drive API** и **Google Picker API** в одном проекте Google Cloud. Создайте OAuth-клиент типа **Desktop app** и ключ API для Picker. Для одного Google-аккаунта используйте [Personal mode](https://console.cloud.google.com/auth/overview/personal). У внешнего приложения в режиме **Testing** refresh token для Drive действует семь дней.

```sh
./bin/s1-material-collector configure-drive \
  --context '<prod-context>' \
  --client-json '<path-to-desktop-client.json>' \
  --picker-project '<google-cloud-project-number>' \
  --parent '<system1-drive-folder-id>'
```

Команда читает ключ Picker из `GOOGLE_PICKER_API_KEY` и выводит ссылку для входа в Google. Войдите аккаунтом с правом добавлять файлы в `System1` и выберите именно эту папку. Команда проверит право записи с обновлённым токеном и передаст Secret в Kubernetes по stdin. OAuth-доступ ограничен scope `drive.file`.

МТС Линк хранит в `s1-material-collector-mts-link` обе cookies и известные сроки их действия. Go-процесс обновляет пару при истечении access cookie или ответе 401 и сразу сохраняет её в этот Secret. Google refresh token и OAuth-клиент хранятся в `s1-material-collector-google-drive`. Google access token используется в памяти Job. Оба Secret создаются отдельно от GitOps и переживают смену Pod.

## Запуск и восстановление

```sh
job="s1-material-collector-manual-$(date +%s)"
kubectl --context '<prod-context>' -n system1 create job "$job" --from=cronjob/s1-material-collector
kubectl --context '<prod-context>' -n system1 logs "job/$job" --all-containers
```

Lease сериализует плановые запуски и ручные Job. Состояние на Drive хранит ID файла, зарезервированный до загрузки. После сбоя между загрузкой и сохранением результата следующий запуск проверит этот ID и завершит запись в `state.json`. При отсутствующем состоянии файлы также распознаются по метаданным с ID исходной расшифровки.

В логах успешной Job для каждой комнаты есть количество найденных, выгруженных, восстановленных, ранее выгруженных и ожидающих файлов. Если расшифровка готовится после окончания встречи, достаточно дождаться следующей проверки или запустить Job повторно.

Prometheus сигнализирует после сбоя Job и через два часа без успешного прогона. Multi-Heart доставляет алёрты в Telegram. При `MTS_LINK_AUTH` выполните `configure-mts` со свежими cookies. При `GOOGLE_DRIVE_AUTH` проверьте право записи в папку и повторите `configure-drive`. При `KUBERNETES_SECRET` проверьте наличие нужного Secret и RBAC. После восстановления запустите Job и проверьте её статус и файл на Drive.

## Релиз

GitHub Actions проверяет Go-код, собирает образ по тегу `v*` и синхронизирует CronJob через ArgoCD. Исключение в `iac/envs/stage/env.yaml` убирает коллектор из списка приложений stage на этапе генерации ApplicationSet. При первом релизе `system1-root` применяет `iac/system1.yaml`. Затем проверьте, что ArgoCD показывает `s1-material-collector-prod`.

Детали Kubernetes общие для планового и ручного запуска. Автоматическое обновление образа применяется к новым Job через `imagePullPolicy: Always`.
