FROM alpine:3.21
WORKDIR /application

COPY /modules/scenario/bin/s1-workflow-validator .
COPY /api/src/workflow-blocks.schema.yaml .
ENV WORKFLOW_CATALOG_PATH=/application/workflow-blocks.schema.yaml

EXPOSE 8080
USER 65534:65534

ENTRYPOINT ["./s1-workflow-validator"]
