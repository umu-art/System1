package handler

import (
	"github.com/labstack/echo/v4"

	"system1/platform/internal/apperr"
	"system1/platform/s1-channels-gateway/service"
)

type Channel struct {
	svc *service.Channel
}

func NewChannel(svc *service.Channel) *Channel {
	return &Channel{svc: svc}
}

func (h *Channel) CreateChannel(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Channel) GetChannel(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Channel) UpdateChannel(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Channel) DeleteChannel(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Channel) ListChannels(_ echo.Context) error {
	return apperr.NotImplemented()
}
