package httpx

import (
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"system1/platform/internal/apperr"
)

// QueryUUID читает обязательный query-параметр в формате UUID.
func QueryUUID(c echo.Context, name string) (uuid.UUID, error) {
	v := c.QueryParam(name)
	if v == "" {
		return uuid.Nil, apperr.Invalid(name + " is required")
	}
	id, err := uuid.Parse(v)
	if err != nil {
		return uuid.Nil, apperr.Invalid(name + " must be a uuid")
	}
	return id, nil
}
