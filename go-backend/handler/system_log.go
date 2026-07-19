package handler

import (
	"flux-panel/go-backend/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

func SystemLogList(c *gin.Context) {
	var d struct {
		Type  string `json:"type"`
		Limit int    `json:"limit"`
	}
	c.ShouldBindJSON(&d)
	c.JSON(http.StatusOK, service.GetSystemLogs(d.Type, d.Limit))
}
