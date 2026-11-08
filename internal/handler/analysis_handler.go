package handler

import (
	"net/http"

	"ila-engine/internal/dto"
	"ila-engine/internal/service"
	"ila-engine/pkg/apperror"
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
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Invalid ILA analysis parameters", err.Error())
		return
	}

	if c.Query("htf") == "" && c.Query("ltf") == "" {
		res, err := h.ilaService.AnalyzeAllPairs(c.Request.Context(), req.Symbol)
		if err != nil {
			status, code := apperror.MapHTTPStatus(err)
			httpx.Error(c, status, code, err.Error(), nil)
			return
		}
		httpx.OK(c, "All 4 ILA paired timeframes analyzed successfully", res)
		return
	}

	res, err := h.ilaService.AnalyzeILA2(c.Request.Context(), req)
	if err != nil {
		status, code := apperror.MapHTTPStatus(err)
		httpx.Error(c, status, code, err.Error(), nil)
		return
	}

	httpx.OK(c, "ILA (Imaginary Liquidity) analysis completed successfully", res)
}
