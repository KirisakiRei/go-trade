package handler

import (
	"net/http"

	"ila-engine/internal/dto"
	"ila-engine/internal/service"
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
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST",
			"Data eksekusi ILA tidak sah", nil)
		return
	}

	res, err := h.executionService.EvaluateAndExecuteILA(c.Request.Context(), req)
	if err != nil {
		httpx.Gagal(c, err, "trade.eksekusi.gagal")
		return
	}

	if !res.Executed {
		httpx.OK(c, "Evaluasi ILA selesai: order ditolak oleh quality gate", res)
		return
	}

	httpx.Created(c, "Posisi ILA berhasil dibuka", res)
}

func (h *TradeHandler) ListPositions(c *gin.Context) {
	var p dto.PaginationQuery
	if err := c.ShouldBindQuery(&p); err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST",
			"Parameter paginasi tidak sah", nil)
		return
	}

	symbol := c.Query("symbol")
	status := c.Query("status")

	positions, meta, err := h.executionService.ListPositions(c.Request.Context(), symbol, status, p)
	if err != nil {
		httpx.Gagal(c, err, "trade.daftar.posisi.gagal")
		return
	}

	httpx.OKWithMeta(c, "Daftar posisi diambil", positions, meta)
}

func (h *TradeHandler) GetAccount(c *gin.Context) {
	acc, err := h.executionService.GetAccount(c.Request.Context())
	if err != nil {
		httpx.Gagal(c, err, "trade.akun.gagal")
		return
	}

	httpx.OK(c, "Informasi akun diambil", acc)
}

func (h *TradeHandler) GetPendingSignals(c *gin.Context) {
	signals := h.executionService.GetPendingSignals()
	httpx.OK(c, "Sinyal tertunda diambil", signals)
}

func (h *TradeHandler) ConfirmSignal(c *gin.Context) {
	var req dto.ConfirmSignalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST",
			"Data konfirmasi tidak sah", nil)
		return
	}

	if err := h.executionService.ConfirmSignal(c.Request.Context(), req); err != nil {
		httpx.Gagal(c, err, "trade.konfirmasi.gagal")
		return
	}

	httpx.OK(c, "Sinyal dikonfirmasi dan dicatat", nil)
}

func (h *TradeHandler) ListEvaluations(c *gin.Context) {
	evals, err := h.evaluationService.ListRecentEvaluations(c.Request.Context(), 20)
	if err != nil {
		httpx.Gagal(c, err, "eval.daftar.gagal")
		return
	}
	httpx.OK(c, "Evaluasi trading diambil", evals)
}

func (h *TradeHandler) GetEvaluationStats(c *gin.Context) {
	stats, err := h.evaluationService.GetEvaluationSummary(c.Request.Context())
	if err != nil {
		httpx.Gagal(c, err, "eval.statistik.gagal")
		return
	}
	httpx.OK(c, "Statistik evaluasi dan pelajaran diambil", stats)
}
