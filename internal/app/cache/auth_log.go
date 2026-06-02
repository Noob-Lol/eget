package cache

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func withBearerToken(next http.Handler, token string) http.Handler {
	token = strings.TrimSpace(token)
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type requestLogEvent struct {
	TS         time.Time `json:"ts"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	Status     int       `json:"status"`
	Bytes      int       `json:"bytes"`
	DurationMS int64     `json:"duration_ms"`
	RemoteAddr string    `json:"remote_addr,omitempty"`
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
		r.ResponseWriter.WriteHeader(status)
	}
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(data)
	r.bytes += n
	return n, err
}

func withJSONLog(next http.Handler, writer io.Writer, now func() time.Time) http.Handler {
	return withRequestLog(next, writer, now, true)
}

func withTextLog(next http.Handler, writer io.Writer, now func() time.Time) http.Handler {
	return withRequestLog(next, writer, now, false)
}

func withRequestLog(next http.Handler, writer io.Writer, now func() time.Time, jsonLog bool) http.Handler {
	if writer == nil {
		writer = os.Stderr
	}
	if now == nil {
		now = time.Now
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		event := requestLogEvent{
			TS:         start,
			Method:     r.Method,
			Path:       r.URL.Path,
			Status:     status,
			Bytes:      rec.bytes,
			DurationMS: now().Sub(start).Milliseconds(),
			RemoteAddr: r.RemoteAddr,
		}
		if jsonLog {
			_ = json.NewEncoder(writer).Encode(event)
			return
		}
		_, _ = fmt.Fprintf(writer, "%s %s %s %d bytes=%d duration_ms=%d remote_addr=%s\n",
			event.TS.Format("01-02 15:04:05"),
			event.Method,
			event.Path,
			event.Status,
			event.Bytes,
			event.DurationMS,
			event.RemoteAddr,
		)
	})
}
