package handler

import (
	"flux-panel/go-backend/dto"
	"flux-panel/go-backend/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GroupCreate(c *gin.Context) {
	var d dto.GroupDto
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusOK, dto.Err("参数错误"))
		return
	}
	c.JSON(http.StatusOK, service.CreateGroup(d))
}

func GroupList(c *gin.Context) {
	c.JSON(http.StatusOK, service.GetAllGroups())
}

func GroupUpdate(c *gin.Context) {
	var d dto.GroupUpdateDto
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusOK, dto.Err("参数错误"))
		return
	}
	c.JSON(http.StatusOK, service.UpdateGroup(d))
}

func GroupDelete(c *gin.Context) {
	var d struct {
		ID int64 `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusOK, dto.Err("参数错误"))
		return
	}
	c.JSON(http.StatusOK, service.DeleteGroup(d.ID))
}

func GroupUpdateOrder(c *gin.Context) {
	var items []dto.OrderItem
	if err := c.ShouldBindJSON(&items); err != nil {
		c.JSON(http.StatusOK, dto.Err("参数错误"))
		return
	}
	c.JSON(http.StatusOK, service.UpdateGroupOrder(items))
}
