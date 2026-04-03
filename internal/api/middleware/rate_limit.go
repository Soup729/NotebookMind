package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type clientLimiter struct {
	limiter    *rate.Limiter
	lastAccess time.Time
}

func RateLimiter(requestsPerSecond float64, burst int) gin.HandlerFunc {
	clients := make(map[string]*clientLimiter)
	var mu sync.Mutex

	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			mu.Lock()
			for ip, entry := range clients {
				if time.Since(entry.lastAccess) > 30*time.Minute {
					delete(clients, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		ip := c.ClientIP()
		mu.Lock()
		entry, ok := clients[ip]
		if !ok {
			entry = &clientLimiter{
				limiter: rate.NewLimiter(rate.Limit(requestsPerSecond), burst),
			}
			clients[ip] = entry
		}
		entry.lastAccess = time.Now()
		mu.Unlock()

		if !entry.limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}

		c.Next()
	}
}
