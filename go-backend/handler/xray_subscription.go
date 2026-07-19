package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"flux-panel/go-backend/dto"
	"flux-panel/go-backend/model"
	"flux-panel/go-backend/service"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// generateRandomToken generates a 32-byte random hex token.
func generateRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func XraySubscription(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.String(http.StatusBadRequest, "invalid token")
		return
	}

	var user model.User
	if err := service.DB.Where("sub_token = ?", token).First(&user).Error; err != nil {
		c.String(http.StatusUnauthorized, "invalid or expired token")
		return
	}

	scope := c.Query("scope")
	result := service.GetSubscriptionLinks(user.ID, scope)
	if result.Code != 0 {
		c.String(http.StatusInternalServerError, result.Msg)
		return
	}

	links, ok := result.Data.([]map[string]interface{})
	if !ok || len(links) == 0 {
		c.String(http.StatusOK, "")
		return
	}

	var linkStrs []string
	for _, item := range links {
		if link, ok := item["link"].(string); ok {
			linkStrs = append(linkStrs, link)
		}
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(strings.Join(linkStrs, "\n")))
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, encoded)
}

func XraySubToken(c *gin.Context) {
	userId := GetUserId(c)
	if userId == 0 {
		c.JSON(http.StatusOK, dto.Err("未登录"))
		return
	}

	// Check Xray permission for non-admin users
	roleId := GetRoleId(c)
	var user model.User
	if err := service.DB.First(&user, userId).Error; err != nil {
		c.JSON(http.StatusOK, dto.Err("用户不存在"))
		return
	}
	if roleId != 0 && user.XrayEnabled != 1 {
		c.JSON(http.StatusOK, dto.Err("你没有 Xray 代理权限"))
		return
	}

	// If no persistent token exists, generate one
	if user.SubToken == "" {
		token, err := generateRandomToken()
		if err != nil {
			c.JSON(http.StatusOK, dto.Err("生成订阅令牌失败"))
			return
		}
		user.SubToken = token
		if err := service.DB.Save(&user).Error; err != nil {
			c.JSON(http.StatusOK, dto.Err("保存订阅令牌失败"))
			return
		}
	}

	c.JSON(http.StatusOK, dto.Ok(map[string]interface{}{
		"token": user.SubToken,
	}))
}

func XraySubReset(c *gin.Context) {
	userId := GetUserId(c)
	if userId == 0 {
		c.JSON(http.StatusOK, dto.Err("未登录"))
		return
	}

	var user model.User
	if err := service.DB.First(&user, userId).Error; err != nil {
		c.JSON(http.StatusOK, dto.Err("用户不存在"))
		return
	}

	token, err := generateRandomToken()
	if err != nil {
		c.JSON(http.StatusOK, dto.Err("生成订阅令牌失败"))
		return
	}
	user.SubToken = token
	if err := service.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusOK, dto.Err("保存订阅令牌失败"))
		return
	}

	c.JSON(http.StatusOK, dto.Ok(map[string]interface{}{
		"token": user.SubToken,
	}))
}

func XraySubLinks(c *gin.Context) {
	userId := GetUserId(c)
	scope := c.Query("scope")
	c.JSON(http.StatusOK, service.GetSubscriptionLinks(userId, scope))
}

func GetSubStore(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.String(http.StatusBadRequest, "missing token")
		return
	}

	var user model.User
	if err := service.DB.Where("sub_token = ?", token).First(&user).Error; err != nil {
		c.String(http.StatusUnauthorized, "invalid token")
		return
	}

	scope := c.Query("scope")
	result := service.GetSubscriptionLinks(user.ID, scope)
	if result.Code != 0 {
		c.String(http.StatusInternalServerError, result.Msg)
		return
	}

	links, ok := result.Data.([]map[string]interface{})
	if !ok || len(links) == 0 {
		c.String(http.StatusOK, "")
		return
	}

	var linkStrs []string
	for _, item := range links {
		if link, ok := item["link"].(string); ok {
			linkStrs = append(linkStrs, link)
		}
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(strings.Join(linkStrs, "\n")))
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, encoded)
}
