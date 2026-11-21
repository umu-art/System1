import os

import uvicorn
from fastapi import FastAPI

from s1_metrics_api.apis.metrics_api import router
from s1_metrics_api.apis.metrics_api_base import BaseMetricsApi
from s1_metrics_api.models.service_info_dto import ServiceInfoDto

SERVICE_NAME = "s1-metrics"


class MetricsApi(BaseMetricsApi):
    async def get_metrics_info(self) -> ServiceInfoDto:
        return ServiceInfoDto(service=SERVICE_NAME, env=os.getenv("S1_ENV", "local"))


app = FastAPI(title=SERVICE_NAME, docs_url=None, redoc_url=None, openapi_url=None)
app.include_router(router)


@app.get("/healthz")
@app.get("/readyz")
def health() -> dict[str, str]:
    return {"status": "ok"}


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=int(os.getenv("PORT", "8080")))
