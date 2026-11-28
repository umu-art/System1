package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"system1/platform/internal/apperr"
	"system1/platform/internal/httpx"
	"system1/platform/s1-dialogs/service"
)

type Client struct {
	svc *service.Client
}

func NewClient(svc *service.Client) *Client {
	return &Client{svc: svc}
}

func (h *Client) CreateClient(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Client) UpdateClient(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Client) GetClient(c echo.Context) error {
	clientID, err := httpx.QueryUUID(c, "id")
	if err != nil {
		return err
	}
	serviceID, err := httpx.QueryUUID(c, "serviceId")
	if err != nil {
		return err
	}
	dto, err := h.svc.Get(c.Request().Context(), clientID, serviceID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, dto)
}

func (h *Client) ListClients(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Client) BlockClient(_ echo.Context) error {
	return apperr.NotImplemented()
}

func (h *Client) UnblockClient(_ echo.Context) error {
	return apperr.NotImplemented()
}
