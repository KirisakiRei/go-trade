package router

import (
	"testing"

	"ila-engine/internal/config"

	"github.com/gin-gonic/gin"
)

func registerRoutesForTest() *gin.Engine {
	return SetupRouter(&config.Config{AllowedOrigins: "*"}, nil).Engine
}

// Rute BBMA harus hilang dan rute ILA harus tetap terdaftar. Kalau tes ini
// gagal, berarti penghapusan BBMA ikut merusak jalur ILA.
func TestRuteHanyaILA(t *testing.T) {
	r := registerRoutesForTest()

	lihat := map[string]bool{}
	for _, rute := range r.Routes() {
		lihat[rute.Method+" "+rute.Path] = true
	}

	hilang := []string{
		"GET /api/v1/analysis/mtf",
		"GET /api/v1/analysis/setups",
		"POST /api/v1/trade/execute",
	}
	for _, rute := range hilang {
		if lihat[rute] {
			t.Errorf("rute BBMA masih terdaftar: %s", rute)
		}
	}

	wajib := []string{
		"GET /api/v1/analysis/ila",
		"POST /api/v1/trade/execute/ila",
		"GET /api/v1/trade/positions",
		"GET /api/v1/trade/account",
	}
	for _, rute := range wajib {
		if !lihat[rute] {
			t.Errorf("rute ILA hilang: %s", rute)
		}
	}
}
