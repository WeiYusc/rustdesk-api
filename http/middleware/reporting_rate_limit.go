package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type reportingRateLimitBucket struct {
	windowStart time.Time
	count       int
}

// ReportingRateLimit limits noisy unauthenticated RustDesk client reporting
// endpoints by client IP and request path. A non-positive limit or window keeps
// the middleware disabled, which preserves compatibility for custom deployments.
func ReportingRateLimit(limit int, window time.Duration) gin.HandlerFunc {
	var mu sync.Mutex
	buckets := make(map[string]reportingRateLimitBucket)

	return func(c *gin.Context) {
		if limit <= 0 || window <= 0 {
			c.Next()
			return
		}

		now := time.Now()
		key := c.ClientIP() + " " + c.FullPath()
		if c.FullPath() == "" {
			key = c.ClientIP() + " " + c.Request.URL.Path
		}

		mu.Lock()
		bucket := buckets[key]
		if bucket.windowStart.IsZero() || now.Sub(bucket.windowStart) >= window {
			bucket = reportingRateLimitBucket{windowStart: now}
		}
		bucket.count++
		buckets[key] = bucket
		limited := bucket.count > limit
		if len(buckets) > 4096 {
			for k, v := range buckets {
				if now.Sub(v.windowStart) >= window {
					delete(buckets, k)
				}
			}
		}
		mu.Unlock()

		if limited {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many reporting requests"})
			c.Abort()
			return
		}
		c.Next()
	}
}
