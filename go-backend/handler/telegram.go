package handler

import (
	"flux-panel/go-backend/dto"
	"flux-panel/go-backend/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

func TelegramStatus(c *gin.Context) {
	userId := c.GetInt64("userId")
	c.JSON(http.StatusOK, service.GetTelegramStatus(userId))
}

func TelegramBindCode(c *gin.Context) {
	userId := c.GetInt64("userId")
	code := service.GenerateTelegramBindCode(userId)
	c.JSON(http.StatusOK, dto.Ok(map[string]string{"code": code}))
}

func TelegramUnbind(c *gin.Context) {
	userId := c.GetInt64("userId")
	c.JSON(http.StatusOK, service.UnbindTelegram(userId))
}
