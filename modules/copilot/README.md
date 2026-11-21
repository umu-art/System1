# copilot

Копилот: диалог с автором и сборка сценариев. Владелец — Леонид. Python.

Сейчас здесь один сервис-шаблон `s1-scenario-builder`. Роуты и модели генерируются из контракта `api/src/s1-scenario-builder.yaml` в пакет `s1_scenario_builder_api`: сервис наследует `Base…Api` и подключает `router`. У каждого сервиса свой `requirements.txt`.

```bash
python ../../api/build.py --langs python
cd s1-scenario-builder
pip install -r requirements.txt
python main.py
curl localhost:8080/api/scenario-builder/info
```

`build.py` ставит сгенерированный пакет в тот Python, которым его запустили, поэтому запускай его из того же окружения, что и сервис.

Переменные: `PORT` (по умолчанию 8080), `S1_ENV` (по умолчанию `local`).
