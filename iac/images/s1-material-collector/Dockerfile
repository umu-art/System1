FROM alpine:3.21
WORKDIR /application

COPY /modules/tools/bin/s1-material-collector .

USER 65534:65534
ENTRYPOINT ["./s1-material-collector"]
