package apperror

import (
	"errors"
	"net/http"
)

var (
	ErrNotFound         = errors.New("resource not found")
	ErrAlreadyExists    = errors.New("resource already exists")
	ErrUnauthorized     = errors.New("unauthorized access")
	ErrForbidden        = errors.New("forbidden resource")
	ErrBadRequest       = errors.New("invalid request payload")
	ErrInternalServer   = errors.New("internal server error")
	ErrInsufficientData = errors.New("insufficient market data")
	ErrInvalidCandle    = errors.New("invalid candlestick data")
	ErrMarketFeedFailed = errors.New("market data feed failed")
)

func MapHTTPStatus(err error) (int, string) {
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, ErrAlreadyExists):
		return http.StatusConflict, "ALREADY_EXISTS"
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized, "UNAUTHORIZED"
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden, "FORBIDDEN"
	case errors.Is(err, ErrBadRequest), errors.Is(err, ErrInvalidCandle):
		return http.StatusBadRequest, "BAD_REQUEST"
	case errors.Is(err, ErrInsufficientData):
		return http.StatusUnprocessableEntity, "INSUFFICIENT_DATA"
	case errors.Is(err, ErrMarketFeedFailed):
		return http.StatusBadGateway, "MARKET_FEED_FAILED"
	default:
		return http.StatusInternalServerError, "INTERNAL_SERVER_ERROR"
	}
}
