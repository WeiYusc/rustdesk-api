package api

import (
	"github.com/gin-gonic/gin"
	requstform "github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"net/http"
	"time"
)

type Index struct {
}

// Index 首页
// @Tags 首页
// @Summary 首页
// @Description 首页
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router / [get]
func (i *Index) Index(c *gin.Context) {
	response.Success(
		c,
		"Hello Gwen",
	)
}

// Heartbeat 心跳
// @Tags 首页
// @Summary 心跳
// @Description 心跳
// @Accept  json
// @Produce  json
// @Success 200 {object} nil
// @Failure 500 {object} response.Response
// @Router /heartbeat [post]
func (i *Index) Heartbeat(c *gin.Context) {
	info := &requstform.PeerInfoInHeartbeat{}
	err := c.ShouldBindJSON(info)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	if info.Uuid == "" {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	peer := service.AllService.PeerService.FindById(info.Id)
	if peer == nil || peer.RowId == 0 {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	resolvedUserId := service.AllService.UserService.FindActiveUserIdByDeviceUuid(peer.Uuid, peer.Id)
	if peer.Uuid != "" && info.Uuid != peer.Uuid {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	if time.Now().Unix()-peer.LastOnlineTime >= 30 {
		upp := &model.Peer{RowId: peer.RowId, UserId: peer.UserId, LastOnlineTime: time.Now().Unix(), LastOnlineIp: c.ClientIP()}
		if resolvedUserId != 0 {
			upp.UserId = resolvedUserId
		}
		if peer.UserId == 0 {
			ab := service.AllService.AddressBookService.InfoByUserIdAndId(1, info.Id) //别名只同步全员地址簿，私人地址簿数据不同步
			if ab != nil && ab.RowId != 0 {
				upp.Alias = ab.Alias
			}
		}
		service.AllService.PeerService.Update(upp)
	}
	c.JSON(http.StatusOK, gin.H{})
}

// Version 版本
// @Tags 首页
// @Summary 版本
// @Description 版本
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /version [get]
func (i *Index) Version(c *gin.Context) {
	//读取resources/version文件
	v := service.AllService.AppService.GetAppVersion()
	response.Success(
		c,
		v,
	)
}

// BuildInfo 构建信息
// @Tags 首页
// @Summary 构建信息
// @Description 构建信息
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /build-info [get]
func (i *Index) BuildInfo(c *gin.Context) {
	response.Success(
		c,
		service.AllService.AppService.GetBuildInfo(),
	)
}
