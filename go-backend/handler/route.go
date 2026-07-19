package handler

import (
	"flux-panel/go-backend/dto"
	"flux-panel/go-backend/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

func RouteCreate(c *gin.Context) {
	var d dto.RouteDto
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusOK, dto.Err("invalid parameters"))
		return
	}
	c.JSON(http.StatusOK, service.CreateRoute(d))
}

func RouteList(c *gin.Context) {
	c.JSON(http.StatusOK, service.GetAllRoutes())
}

func RouteUpdate(c *gin.Context) {
	var d dto.RouteUpdateDto
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusOK, dto.Err("invalid parameters"))
		return
	}
	c.JSON(http.StatusOK, service.UpdateRoute(d))
}

func RouteDelete(c *gin.Context) {
	var d struct {
		ID int64 `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusOK, dto.Err("invalid parameters"))
		return
	}
	c.JSON(http.StatusOK, service.DeleteRoute(d.ID))
}
