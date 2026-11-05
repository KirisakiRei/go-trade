package handler

import (
	"net/http"

	"ila-engine/internal/service"
	"ila-engine/pkg/httpx"

	"github.com/gin-gonic/gin"
)

type MonitorHandler struct {
	monitorService service.MonitorService
}

func NewMonitorHandler(monitorService service.MonitorService) *MonitorHandler {
	return &MonitorHandler{monitorService: monitorService}
}

type StartMonitorRequest struct {
	Symbol    string `json:"symbol" binding:"required"`
	ContextTF string `json:"context_tf" binding:"required"`
	EntryTF   string `json:"entry_tf" binding:"required"`
}

func (h *MonitorHandler) Start(c *gin.Context) {
	var req StartMonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Invalid start monitor request", err.Error())
		return
	}

	err := h.monitorService.StartMonitoring(c.Request.Context(), req.Symbol, req.ContextTF, req.EntryTF)
	if err != nil {
		httpx.Error(c, http.StatusConflict, "ALREADY_RUNNING", err.Error(), nil)
		return
	}

	httpx.OK(c, "Monitor started successfully", h.monitorService.GetStatus())
}

func (h *MonitorHandler) Stop(c *gin.Context) {
	h.monitorService.StopMonitoring()
	httpx.OK(c, "Monitor stopped successfully", h.monitorService.GetStatus())
}

func (h *MonitorHandler) Status(c *gin.Context) {
	status := h.monitorService.GetStatus()
	httpx.OK(c, "Monitor status retrieved", status)
}
