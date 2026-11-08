package handler

import (
	"net/http"

	"ila-engine/internal/dto"
	"ila-engine/internal/service"
	"ila-engine/pkg/httpx"

	"github.com/gin-gonic/gin"
)

type AnalysisHandler struct {
	ilaService service.ILA2Service
}

func NewAnalysisHandler(ilaService service.ILA2Service) *AnalysisHandler {
	return &AnalysisHandler{ilaService: ilaService}
}

func (h *AnalysisHandler) AnalyzeILA2(c *gin.Context) {
	var req dto.ILA2AnalysisRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST",
			"Parameter analisis ILA tidak sah", nil)
		return
	}

	if c.Query("htf") == "" && c.Query("ltf") == "" {
		res, err := h.ilaService.AnalyzeAllPairs(c.Request.Context(), req.Symbol)
		if err != nil {
			httpx.Gagal(c, err, "analysis.pairs.gagal")
			return
		}
		httpx.OK(c, "Seluruh 4 pasangan timeframe ILA dianalisis", res)
		return
	}

	res, err := h.ilaService.AnalyzeILA2(c.Request.Context(), req)
	if err != nil {
		httpx.Gagal(c, err, "analysis.ila.gagal")
		return
	}

	httpx.OK(c, "Analisis ILA (Imaginary Liquidity) selesai", res)
}
