package http

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/middleware"
	"github.com/lejianwen/rustdesk-api/v2/http/router"
	"github.com/sirupsen/logrus"
	"net/http"
	"strings"
)

const bytesPerMegabyte = 1024 * 1024

func configureTrustedProxies(g *gin.Engine, trustProxy string) error {
	// Empty trust-proxy means do not trust forwarded proxy headers. Gin's default
	// behavior can otherwise trust proxy headers too broadly, which makes
	// ClientIP-based login limits spoofable on directly exposed deployments.
	if trustProxy == "" {
		return g.SetTrustedProxies(nil)
	}
	pro := strings.Split(trustProxy, ",")
	return g.SetTrustedProxies(pro)
}

func ApiInit() {
	gin.SetMode(global.Config.Gin.Mode)
	g := gin.New()

	//[WARNING] You trusted all proxies, this is NOT safe. We recommend you to set a value.
	//Please check https://pkg.go.dev/github.com/gin-gonic/gin#readme-don-t-trust-all-proxies for details.
	if err := configureTrustedProxies(g, global.Config.Gin.TrustProxy); err != nil {
		panic(err)
	}

	if global.Config.Gin.Mode == gin.ReleaseMode {
		//修改gin Recovery日志 输出为logger的输出点
		if global.Logger != nil {
			gin.DefaultErrorWriter = global.Logger.WriterLevel(logrus.ErrorLevel)
		}
	}
	g.NoRoute(func(c *gin.Context) {
		c.String(http.StatusNotFound, "404 not found")
	})
	g.Use(middleware.Logger(), middleware.Limiter(), middleware.BodyLimit(global.Config.Gin.BodyMaxSizeMb*bytesPerMegabyte, "/api/admin/file/upload"), gin.Recovery())
	router.WebInit(g)
	router.Init(g)
	router.ApiInit(g)
	Run(g, global.Config.Gin.ApiAddr)
}
