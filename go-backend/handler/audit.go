package handler

import (
	"flux-panel/go-backend/dto"
	"flux-panel/go-backend/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

func AuditUpload(c *gin.Context) {
	secret := getNodeSecret(c)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxFlowBodySize)
	body, err := readRequestBody(c)
	if err != nil {
		c.String(http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	c.String(http.StatusOK, service.ProcessConnectionAuditUpload(string(body), secret))
}

func AuditList(c *gin.Context) {
	var d dto.ConnectionAuditListDto
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusOK, dto.Err("invalid parameters"))
		return
	}
	c.JSON(http.StatusOK, service.GetConnectionAudits(d))
}
