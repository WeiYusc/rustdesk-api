package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
)

func parsePositiveIDParam(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 0)
	if err != nil || id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return 0, false
	}
	return uint(id), true
}
