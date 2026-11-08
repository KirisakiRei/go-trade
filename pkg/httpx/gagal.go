package httpx

import (
	"errors"
	"log/slog"
	"net/http"

	"ila-engine/pkg/apperror"

	"github.com/gin-gonic/gin"
)

// Gagal mencatat error asli ke log lalu menjawab klien dengan pesan aman.
//
// Detail internal (nama host, path berkas, galat driver) tidak boleh pernah
// sampai ke pemanggil HTTP; cukup masuk log.
func Gagal(c *gin.Context, err error, evt string) {
	status, kode := apperror.MapHTTPStatus(err)

	slog.Error("permintaan gagal",
		"evt", evt,
		"status", status,
		"error", err.Error(),
		"path", c.FullPath(),
	)

	pesan := pesanAman(err, status)
	Error(c, status, kode, pesan, nil)
}

// pesanAman memilih pesan yang boleh dibaca pemanggil.
func pesanAman(err error, status int) string {
	switch {
	case errors.Is(err, apperror.ErrNotFound):
		return "Data tidak ditemukan"
	case errors.Is(err, apperror.ErrBadRequest), errors.Is(err, apperror.ErrInvalidCandle):
		return "Permintaan tidak sah"
	case errors.Is(err, apperror.ErrInsufficientData):
		return "Data pasar belum cukup untuk analisis"
	case errors.Is(err, apperror.ErrMarketFeedFailed):
		return "Sumber data pasar sedang tidak tersedia"
	}

	if status < http.StatusInternalServerError {
		return http.StatusText(status)
	}
	return "Terjadi kesalahan pada server"
}
