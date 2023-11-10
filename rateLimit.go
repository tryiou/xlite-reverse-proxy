// rate limiting for clients

package main

import (
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// Create a custom visitor struct which holds the rate limiter for each
// visitor and the last time that the visitor was seen.
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// Change the the map to hold values of the type visitor.
var visitors = make(map[string]*visitor)

// Run a background goroutine to remove old entries from the visitors map.
func init() {
	go cleanupVisitors()
}

func getVisitor(ip string) *rate.Limiter {
	mu.Lock()
	defer mu.Unlock()

	v, exists := visitors[ip]
	if !exists {
		limiter := rate.NewLimiter(rate.Every(time.Minute/time.Duration(rateLimit)), rateLimit)
		// Include the current time when creating a new visitor.
		visitors[ip] = &visitor{limiter, time.Now()}
		return limiter
	}

	// Update the last seen time for the visitor.
	v.lastSeen = time.Now()
	return v.limiter
}

// Every minute check the map for visitors that haven't been seen for
// more than 3 minutes and delete the entries.
func cleanupVisitors() {
	for {
		time.Sleep(time.Minute)

		mu.Lock()
		for ip, v := range visitors {
			if time.Since(v.lastSeen) > 3*time.Minute {
				delete(visitors, ip)
			}
		}
		mu.Unlock()
	}
}

/*
func limit_old(next http.Handler) http.Handler {  // hard rate limit
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// ip, _, err := net.SplitHostPort(r.RemoteAddr)
		// if err != nil {
		// 	log.Print(err.Error())
		// 	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		// 	return
		// }

		var ip string
		var err error
		reqClientIP := r.Header.Get("X-Forwarded-For")
		if reqClientIP != "" {
			// The client IP is available in the X-Forwarded-For header
			// Parse it if there are multiple IPs separated by commas
			ips := strings.Split(reqClientIP, ",")
			ip = strings.TrimSpace(ips[0])
			// Use the clientIP variable as needed
		} else {
			// The X-Forwarded-For header is not present, fallback to previous method
			ip, _, err = net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				logger.Printf("error extracting client ip from request: %v", err)
				return
			}
		}

		limiter := getVisitor(ip)
		if !limiter.Allow() {
			logger.Printf("Rate limit exceeded for IP: %s", ip)
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}*/

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
				logger.Printf("error extracting client ip from request: %v", err)
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
