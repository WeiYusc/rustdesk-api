package api

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	request "github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"gorm.io/gorm"
	"net/http"
	"time"
)

type Audit struct {
}

// AuditConn
// @Tags 审计
// @Summary 审计连接
// @Description 审计连接
// @Accept  json
// @Produce  json
// @Param body body request.AuditConnForm true "审计连接"
// @Success 200 {string} string ""
// @Failure 500 {object} response.Response
// @Router /audit/conn [post]
func (a *Audit) AuditConn(c *gin.Context) {
	af := &request.AuditConnForm{}
	err := c.ShouldBindBodyWith(af, binding.JSON)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	/*ttt := &gin.H{}
	c.ShouldBindBodyWith(ttt, binding.JSON)
	fmt.Println(ttt)*/
	ac := af.ToAuditConn()
	if af.Id == "" || (af.Action == model.AuditActionNew && af.ConnId == 0) {
		response.Error(c, response.TranslateMsg(c, "ParamsError"))
		return
	}
	if af.Action == model.AuditActionNew {
		if err := service.AllService.AuditService.CreateAuditConn(ac); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "audit temporarily unavailable"})
			return
		}
	} else if af.Action == model.AuditActionClose {
		ex, err := service.AllService.AuditService.FindAuditConnByPeerIdAndConnId(af.Id, af.ConnId)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "audit temporarily unavailable"})
			return
		}
		if ex.Id != 0 {
			ex.CloseTime = time.Now().Unix()
			if err := service.AllService.AuditService.UpdateAuditConn(ex); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "audit temporarily unavailable"})
				return
			}
		}
	} else if af.Action == "" {
		ex, err := service.AllService.AuditService.FindAuditConnByPeerIdAndConnId(af.Id, af.ConnId)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "audit temporarily unavailable"})
			return
		}
		if ex.Id != 0 {
			up := &model.AuditConn{
				IdModel:   model.IdModel{Id: ex.Id},
				FromPeer:  ac.FromPeer,
				FromName:  ac.FromName,
				SessionId: ac.SessionId,
				Type:      ac.Type,
			}
			if err := service.AllService.AuditService.UpdateAuditConn(up); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "audit temporarily unavailable"})
				return
			}
		}
	}
	c.Status(http.StatusOK)
}

// AuditFile
// @Tags 审计
// @Summary 审计文件
// @Description 审计文件
// @Accept  json
// @Produce  json
// @Param body body request.AuditFileForm true "审计文件"
// @Success 200 {string} string ""
// @Failure 500 {object} response.Response
// @Router /audit/file [post]
func (a *Audit) AuditFile(c *gin.Context) {
	aff := &request.AuditFileForm{}
	err := c.ShouldBindBodyWith(aff, binding.JSON)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	//ttt := &gin.H{}
	//c.ShouldBindBodyWith(ttt, binding.JSON)
	//fmt.Println(ttt)
	af := aff.ToAuditFile()
	if aff.Id == "" {
		response.Error(c, response.TranslateMsg(c, "ParamsError"))
		return
	}
	if err := service.AllService.AuditService.CreateAuditFile(af); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "audit temporarily unavailable"})
		return
	}
	c.Status(http.StatusOK)
}
