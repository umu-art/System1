FROM flyway/flyway:11.20

ENV FLYWAY_DEFAULT_SCHEMA=public
COPY db/postgres/migrations/ /flyway/sql/

CMD ["migrate"]
