package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"net/http"
)

func Limiter(skipPaths ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, skipPath := range skipPaths {
			if c.Request.URL.Path == skipPath {
				c.Next()
				return
			}
		}
		loginLimiter := global.LoginLimiter
		if loginLimiter == nil {
			c.Next()
			return
		}
		clientIp := c.ClientIP()
		banned, _ := loginLimiter.CheckSecurityStatus(clientIp)
		if banned {
			response.Fail(c, http.StatusLocked, response.TranslateMsg(c, "Banned"))
			c.Abort()
			return
		}
		c.Next()
	}
}
