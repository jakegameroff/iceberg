package main

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Per-IP limits: 5 requests/sec sustained, bursts up to 20.
const (
	rlRate  = rate.Limit(5)
	rlBurst = 20
)

type ipLimiter struct {
	lim  *rate.Limiter
	seen time.Time
}

var (
	rlMu      sync.Mutex
	rlClients = map[string]*ipLimiter{}
)

// rateLimit returns true if the request may proceed. If not, it writes a 429
// (with Retry-After) and returns false; the caller should just return.
func rateLimit(w http.ResponseWriter, r *http.Request) bool {
	ip := clientIP(r)

	rlMu.Lock()
	c, ok := rlClients[ip]
	if !ok {
		c = &ipLimiter{lim: rate.NewLimiter(rlRate, rlBurst)}
		rlClients[ip] = c
	}
	c.seen = time.Now()
	rlMu.Unlock()

	// Reserve a token; if it isn't available right now, reject and hand it back.
	res := c.lim.Reserve()
	if delay := res.Delay(); delay > 0 {
		res.Cancel()
		w.Header().Set("Retry-After", strconv.Itoa(int(delay.Seconds())+1))
		http.Error(w, "rate limited... slow down", http.StatusTooManyRequests)
		return false
	}
	return true
}

// clientIP returns the client's IP from the socket peer (RemoteAddr).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// cleanLimiters drops IPs we haven't seen in a while so the map stays bounded.
func cleanLimiters() {
	rlMu.Lock()
	defer rlMu.Unlock()
	cutoff := time.Now().Add(-15 * time.Minute)
	for ip, c := range rlClients {
		if c.seen.Before(cutoff) {
			delete(rlClients, ip)
		}
	}
}

func startLimiterCleaner(every time.Duration) {
	go func() {
		for {
			time.Sleep(every)
			cleanLimiters()
		}
	}()
}
