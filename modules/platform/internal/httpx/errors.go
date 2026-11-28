package httpx

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"system1/platform/internal/apperr"
)

const internalError = "internal error"

type errorBody struct {
	Message string `json:"message"`
}

func kindStatus(k apperr.Kind) int {
	switch k {
	case apperr.KindInvalid:
		return http.StatusBadRequest
	case apperr.KindUnauthenticated:
		return http.StatusUnauthorized
	case apperr.KindForbidden:
		return http.StatusForbidden
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindConflict:
		return http.StatusConflict
	case apperr.KindNotImplemented:
		return http.StatusNotImplemented
	default:
		return http.StatusInternalServerError
	}
}

func handleError(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}
	status, message := statusOf(err)
	if c.Request().Method == http.MethodHead {
		_ = c.NoContent(status)
		return
	}
	_ = c.JSON(status, errorBody{Message: message})
}

func statusOf(err error) (int, string) {
	status, message := http.StatusInternalServerError, internalError
	if ae, ok := apperr.As(err); ok {
		status, message = kindStatus(ae.Kind), ae.Message
	}
	var he *echo.HTTPError
	if errors.As(err, &he) {
		status, message = he.Code, fmt.Sprint(he.Message)
	}
	if status >= http.StatusInternalServerError {
		message = internalError
	}
	return status, message
}
