package main

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	dialogsroutes "s1_dialogs_api/handlers"
	"system1/platform/s1-dialogs/handler"
	"system1/platform/s1-dialogs/service"
	"system1/platform/s1-dialogs/store/sqlc"
)

func registerRoutes(e *echo.Echo, pool *pgxpool.Pool) {
	q := sqlc.New(pool)
	dialogsroutes.RouteClientAPI(e, handler.NewClient(service.NewClient(q)))
	dialogsroutes.RouteDialogAPI(e, handler.NewDialog(service.NewDialog(q)))
	dialogsroutes.RouteMessageAPI(e, handler.NewMessage(service.NewMessage(q)))
}
