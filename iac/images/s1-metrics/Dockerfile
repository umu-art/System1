FROM python:3.14-slim
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
WORKDIR /application

COPY /modules/analytics/s1-metrics/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY /api/build/python-s1-metrics-api/dist/ /tmp/api/
RUN pip install --no-cache-dir --no-deps /tmp/api/*.whl && rm -rf /tmp/api

COPY /modules/analytics/s1-metrics/main.py .

EXPOSE 8080
USER 65534:65534

ENTRYPOINT ["python", "main.py"]
