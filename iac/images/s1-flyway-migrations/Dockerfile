FROM flyway/flyway:11.20.0@sha256:60fa17a11de73c3a7c3ec9cfda75ac1dcbe77b9f4d5240e31b63321c7466c557

COPY iac/images/s1-flyway-migrations/flyway.conf /flyway/conf/flyway.conf
COPY db/postgres/migrations/ /flyway/sql/

CMD ["migrate"]
