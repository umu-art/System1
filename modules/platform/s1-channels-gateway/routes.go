package main

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	gatewayroutes "s1_channels_gateway_api/handlers"
	"system1/platform/s1-channels-gateway/handler"
	"system1/platform/s1-channels-gateway/service"
	"system1/platform/s1-channels-gateway/store/sqlc"
)

func registerRoutes(e *echo.Echo, pool *pgxpool.Pool) {
	q := sqlc.New(pool)
	gatewayroutes.RouteChannelAPI(e, handler.NewChannel(service.NewChannel(q)))
}
