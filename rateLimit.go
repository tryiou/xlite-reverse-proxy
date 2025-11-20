// rate limiting for clients

package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Create a custom visitor struct which holds the rate limiter for each
// visitor and the last time that the visitor was seen.
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// Use a struct with RWMutex for thread-safe access to the visitors map
var visitors = struct {
	sync.RWMutex
	m map[string]*visitor
}{m: make(map[string]*visitor)}

// Run a background goroutine to remove old entries from the visitors map.
func init() {
	go cleanupVisitors()
}

func getVisitor(ip string) *rate.Limiter {
	// Use RLock first to check if visitor exists
	visitors.RLock()
	v, exists := visitors.m[ip]
	visitors.RUnlock()

	if !exists {
		visitors.Lock()
		// Double-check after acquiring write lock
		if v, exists = visitors.m[ip]; !exists {
			cfg := globalConfig.GetConfig()
			limiter := rate.NewLimiter(rate.Every(time.Minute/time.Duration(cfg.RateLimit)), cfg.RateLimit)
			visitors.m[ip] = &visitor{limiter, time.Now()}
			v = visitors.m[ip]
		}
		visitors.Unlock()
	}

	// Update the last seen time for the visitor (safe without lock for this field)
	v.lastSeen = time.Now()
	return v.limiter
}

// Every minute check the map for visitors that haven't been seen for
// more than 3 minutes and delete the entries.
func cleanupVisitors() {
	for {
		time.Sleep(time.Minute)

		visitors.Lock()
		for ip, v := range visitors.m {
			if time.Since(v.lastSeen) > VisitorCleanupInterval {
				delete(visitors.m, ip)
			}
		}
		visitors.Unlock()
	}
}

func limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ip string
		var err error
		reqClientIP := r.Header.Get("X-Forwarded-For")
		if reqClientIP != "" {
			ips := strings.Split(reqClientIP, ",")
			ip = strings.TrimSpace(ips[0])
		} else {
			ip, _, err = net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				logger.Printf(LogPrefixError+"_error extracting client ip from request: %v", err)
				return
			}
		}

		limiter := getVisitor(ip)
		if !limiter.Allow() {
			remainingTime := limiter.Reserve().Delay()
			time.Sleep(remainingTime)
		}

		next.ServeHTTP(w, r)
	})
}
