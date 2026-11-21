FROM alpine:3.21
WORKDIR /application

COPY /modules/scenario/bin/s1-workflow .

EXPOSE 8080
USER 65534:65534

ENTRYPOINT ["./s1-workflow"]
