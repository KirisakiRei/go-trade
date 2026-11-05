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
	bbmaService service.BBMAService
	ilaService  service.ILAService
}

func NewAnalysisHandler(bbmaService service.BBMAService, ilaService service.ILAService) *AnalysisHandler {
	return &AnalysisHandler{
		bbmaService: bbmaService,
		ilaService:  ilaService,
	}
}

func (h *AnalysisHandler) Analyze(c *gin.Context) {
	var req dto.AnalysisRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Invalid analysis query parameters", err.Error())
		return
	}

	res, err := h.bbmaService.Analyze(c.Request.Context(), req)
	if err != nil {
		status, code := apperror.MapHTTPStatus(err)
		httpx.Error(c, status, code, err.Error(), nil)
		return
	}

	httpx.OK(c, "Analysis completed successfully", res)
}

func (h *AnalysisHandler) ListSetups(c *gin.Context) {
	var p dto.PaginationQuery
	if err := c.ShouldBindQuery(&p); err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Invalid pagination parameters", err.Error())
		return
	}

	symbol := c.Query("symbol")
	setups, meta, err := h.bbmaService.ListActiveSetups(c.Request.Context(), symbol, p)
	if err != nil {
		status, code := apperror.MapHTTPStatus(err)
		httpx.Error(c, status, code, err.Error(), nil)
		return
	}

	httpx.OKWithMeta(c, "Active setups retrieved successfully", setups, meta)
}

func (h *AnalysisHandler) AnalyzeMTF(c *gin.Context) {
	var req dto.MTFAnalysisRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Invalid MTF analysis parameters", err.Error())
		return
	}

	res, err := h.bbmaService.AnalyzeMTF(c.Request.Context(), req)
	if err != nil {
		status, code := apperror.MapHTTPStatus(err)
		httpx.Error(c, status, code, err.Error(), nil)
		return
	}

	httpx.OK(c, "Multi-timeframe analysis completed successfully", res)
}

func (h *AnalysisHandler) AnalyzeILA(c *gin.Context) {
	var req dto.ILAAnalysisRequest
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

	res, err := h.ilaService.AnalyzeILA(c.Request.Context(), req)
	if err != nil {
		status, code := apperror.MapHTTPStatus(err)
		httpx.Error(c, status, code, err.Error(), nil)
		return
	}

	httpx.OK(c, "ILA (Imaginary Liquidity) analysis completed successfully", res)
}
