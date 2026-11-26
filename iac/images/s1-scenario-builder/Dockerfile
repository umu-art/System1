FROM python:3.14-slim
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
WORKDIR /application

COPY /modules/copilot/s1-scenario-builder/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY /api/build/python-s1-scenario-builder-api/dist/ /tmp/api/
RUN pip install --no-cache-dir --no-deps /tmp/api/*.whl && rm -rf /tmp/api

COPY /modules/copilot/s1-scenario-builder/main.py .
COPY /modules/copilot/s1-scenario-builder/agent.py /modules/copilot/s1-scenario-builder/beta.py /modules/copilot/s1-scenario-builder/beta.html ./
COPY /modules/copilot/s1-scenario-builder/prompts/ ./prompts/

EXPOSE 8080
USER 65534:65534

ENTRYPOINT ["python", "main.py"]
