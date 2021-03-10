package main

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	header      string
}

func wrapResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w}
}

func (rw *responseWriter) Status() int {
	return rw.status
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}

	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
	rw.wroteHeader = true

	return
}

// Logs the incoming HTTP request and part of response
func LoggingMiddleware(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				log.Printf("err %v, %v", err, debug.Stack())
			}
		}()

		start := time.Now()
		wrapped := wrapResponseWriter(w)
		next.ServeHTTP(wrapped, r)
		logHTTPReq(wrapped, r, start)
	})
}

func logHTTPReq(w *responseWriter, r *http.Request, startTime time.Time) {
	if skipHTTPRequestLogging(r) {
		return
	}
	logentry := fmt.Sprintf("%s %s \"%s %s\" %d %v",
		r.RemoteAddr,
		r.Host,
		r.Method,
		r.URL.EscapedPath(),
		w.status,
		time.Since(startTime))
	if r.Header.Get("Referer") != "" {
		logentry += fmt.Sprintf(" \"Referer: %s\"", r.Header.Get("Referer"))
	}
	if r.Header.Get("x-original-uri") != "" {
		logentry += fmt.Sprintf(" \"x-original-uri: %s\"", r.Header.Get("x-original-uri"))
	}
	if w.Header().Get("X-Accel-Redirect") != "" {
		logentry += fmt.Sprintf(" \"x-Redirect: %s\"", w.Header().Get("X-Accel-Redirect"))
	}
	log.Println(logentry)
}

// Checks to skip logging some requests
func skipHTTPRequestLogging(r *http.Request) bool {
	switch r.URL.String() {
	case "/favicon.png":
		return true
	case "/favicon.ico":
		return true
	case "/health":
		return true
	}
	if strings.HasPrefix(r.URL.String(), "/favicon-") {
		return true
	}
	return false
}