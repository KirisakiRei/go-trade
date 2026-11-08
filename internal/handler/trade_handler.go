package handler

import (
	"net/http"

	"ila-engine/internal/dto"
	"ila-engine/internal/service"
	"ila-engine/pkg/apperror"
	"ila-engine/pkg/httpx"

	"github.com/gin-gonic/gin"
)

type TradeHandler struct {
	executionService  service.ExecutionService
	evaluationService service.EvaluationService
}

func NewTradeHandler(executionService service.ExecutionService, evaluationService service.EvaluationService) *TradeHandler {
	return &TradeHandler{
		executionService:  executionService,
		evaluationService: evaluationService,
	}
}

func (h *TradeHandler) ExecuteILA(c *gin.Context) {
	var req dto.ILA2ExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Invalid ILA trade execution payload", err.Error())
		return
	}

	res, err := h.executionService.EvaluateAndExecuteILA(c.Request.Context(), req)
	if err != nil {
		status, code := apperror.MapHTTPStatus(err)
		httpx.Error(c, status, code, err.Error(), nil)
		return
	}

	if !res.Executed {
		httpx.OK(c, "ILA trade evaluation completed: order rejected by quality gate", res)
		return
	}

	httpx.Created(c, "ILA position successfully opened", res)
}

func (h *TradeHandler) ListPositions(c *gin.Context) {
	var p dto.PaginationQuery
	if err := c.ShouldBindQuery(&p); err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Invalid pagination query", err.Error())
		return
	}

	symbol := c.Query("symbol")
	status := c.Query("status")

	positions, meta, err := h.executionService.ListPositions(c.Request.Context(), symbol, status, p)
	if err != nil {
		statusErr, code := apperror.MapHTTPStatus(err)
		httpx.Error(c, statusErr, code, err.Error(), nil)
		return
	}

	httpx.OKWithMeta(c, "Positions retrieved successfully", positions, meta)
}

func (h *TradeHandler) GetAccount(c *gin.Context) {
	acc, err := h.executionService.GetAccount(c.Request.Context())
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get account info", err.Error())
		return
	}

	httpx.OK(c, "Account info retrieved", acc)
}

func (h *TradeHandler) GetPendingSignals(c *gin.Context) {
	signals := h.executionService.GetPendingSignals()
	httpx.OK(c, "Pending signals retrieved", signals)
}

func (h *TradeHandler) ConfirmSignal(c *gin.Context) {
	var req dto.ConfirmSignalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Invalid confirmation payload", err.Error())
		return
	}

	if err := h.executionService.ConfirmSignal(c.Request.Context(), req); err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error(), nil)
		return
	}

	httpx.OK(c, "Signal confirmed and recorded", nil)
}

func (h *TradeHandler) ListEvaluations(c *gin.Context) {
	evals, err := h.evaluationService.ListRecentEvaluations(c.Request.Context(), 20)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list evaluations", err.Error())
		return
	}
	httpx.OK(c, "Trade evaluations retrieved successfully", evals)
}

func (h *TradeHandler) GetEvaluationStats(c *gin.Context) {
	stats, err := h.evaluationService.GetEvaluationSummary(c.Request.Context())
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get evaluation stats", err.Error())
		return
	}
	httpx.OK(c, "Evaluation statistics and lessons retrieved", stats)
}
