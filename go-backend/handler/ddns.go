package handler

import (
	"flux-panel/go-backend/dto"
	"flux-panel/go-backend/pkg/ddns"
	"flux-panel/go-backend/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

func DdnsProviderTypes(c *gin.Context) {
	c.JSON(http.StatusOK, dto.Ok(ddns.SupportedTypes()))
}

func DdnsProviderCreate(c *gin.Context) {
	var d dto.DdnsProviderDto
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusOK, dto.Err("参数错误"))
		return
	}
	c.JSON(http.StatusOK, service.CreateDdnsProvider(d))
}

func DdnsProviderList(c *gin.Context) {
	c.JSON(http.StatusOK, service.GetAllDdnsProviders())
}

func DdnsProviderUpdate(c *gin.Context) {
	var d dto.DdnsProviderUpdateDto
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusOK, dto.Err("参数错误"))
		return
	}
	c.JSON(http.StatusOK, service.UpdateDdnsProvider(d))
}

func DdnsProviderDelete(c *gin.Context) {
	var d struct {
		ID int64 `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusOK, dto.Err("参数错误"))
		return
	}
	c.JSON(http.StatusOK, service.DeleteDdnsProvider(d.ID))
}

func DdnsDomainCreate(c *gin.Context) {
	var d dto.DdnsDomainDto
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusOK, dto.Err("参数错误"))
		return
	}
	c.JSON(http.StatusOK, service.CreateDdnsDomain(d))
}

func DdnsDomainList(c *gin.Context) {
	c.JSON(http.StatusOK, service.GetAllDdnsDomains())
}

func DdnsDomainUpdate(c *gin.Context) {
	var d dto.DdnsDomainUpdateDto
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusOK, dto.Err("参数错误"))
		return
	}
	c.JSON(http.StatusOK, service.UpdateDdnsDomain(d))
}

func DdnsDomainDelete(c *gin.Context) {
	var d struct {
		ID int64 `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusOK, dto.Err("参数错误"))
		return
	}
	c.JSON(http.StatusOK, service.DeleteDdnsDomain(d.ID))
}

func DdnsDomainResolve(c *gin.Context) {
	var d struct {
		ID int64 `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusOK, dto.Err("参数错误"))
		return
	}
	c.JSON(http.StatusOK, service.ResolveDdnsDomain(d.ID))
}
