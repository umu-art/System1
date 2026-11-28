package handler

import (
	"github.com/labstack/echo/v4"

	"system1/platform/internal/apperr"
	"system1/platform/s1-dialogs/service"
)

type Dialog struct {
	svc *service.Dialog
}

func NewDialog(svc *service.Dialog) *Dialog {
	return &Dialog{svc: svc}
}

func (h *Dialog) CreateDialog(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Dialog) GetDialog(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Dialog) DeleteDialog(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Dialog) ListDialogs(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Dialog) MarkDialogClientBlocked(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Dialog) MarkDialogClientUnblocked(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Dialog) RemoveDialogs(_ echo.Context) error {
	return apperr.NotImplemented()
}
