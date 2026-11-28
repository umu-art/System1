package handler

import (
	"github.com/labstack/echo/v4"

	"system1/platform/internal/apperr"
	"system1/platform/s1-dialogs/service"
)

type Message struct {
	svc *service.Message
}

func NewMessage(svc *service.Message) *Message {
	return &Message{svc: svc}
}

func (h *Message) CreateMessage(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Message) GetMessage(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Message) ListMessages(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Message) ReportMessageDelivery(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Message) TakeMessage(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Message) MarkMessageProcessed(_ echo.Context) error {
	return apperr.NotImplemented()
}
