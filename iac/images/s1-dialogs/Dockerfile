FROM alpine:3.21
WORKDIR /application

COPY /modules/platform/bin/s1-dialogs .

EXPOSE 8080
USER 65534:65534

ENTRYPOINT ["./s1-dialogs"]
