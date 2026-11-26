import os

import uvicorn
from fastapi import FastAPI

from s1_scenario_builder_api.apis.scenario_builder_api import router
from s1_scenario_builder_api.apis.scenario_builder_api_base import BaseScenarioBuilderApi
from s1_scenario_builder_api.models.service_info_dto import ServiceInfoDto

SERVICE_NAME = "s1-scenario-builder"


class ScenarioBuilderApi(BaseScenarioBuilderApi):
    async def get_scenario_builder_info(self) -> ServiceInfoDto:
        return ServiceInfoDto(service=SERVICE_NAME, env=os.getenv("S1_ENV", "local"))


app = FastAPI(title=SERVICE_NAME, docs_url=None, redoc_url=None, openapi_url=None)
app.include_router(router)

env = os.getenv("S1_ENV", "local")
if env in ("local", "stage"):
    from beta import router as beta_router

    app.include_router(beta_router, prefix="/api/scenario-builder" if env == "stage" else "")


@app.get("/healthz")
@app.get("/readyz")
def health() -> dict[str, str]:
    return {"status": "ok"}


if __name__ == "__main__":
    host = "127.0.0.1" if os.getenv("S1_ENV", "local") == "local" else "0.0.0.0"
    uvicorn.run(app, host=host, port=int(os.getenv("PORT", "8080")))
