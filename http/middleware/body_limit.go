package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// BodyLimit caps request body reads for normal API requests. Handlers that read
// more than maxBytes receive http.ErrBodyReadAfterClose / MaxBytesError from the
// standard library instead of allowing unbounded memory use. skipPrefixes are
// left to route-specific handlers such as multipart upload, which have their own
// size validation.
func BodyLimit(maxBytes int64, skipPrefixes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, prefix := range skipPrefixes {
			if strings.HasPrefix(c.Request.URL.Path, prefix) {
				c.Next()
				return
			}
		}
		if maxBytes > 0 && c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}
