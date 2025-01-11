package handler

import (
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/albertb/putarr/internal/arr"
	"github.com/albertb/putarr/internal/config"
	"github.com/albertb/putarr/internal/putio"
)

func New(options *config.Options, putioClient *putio.Client, arrClient *arr.Client) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", http.NotFoundHandler())

	status := newStatusHandler(options, putioClient, arrClient)
	status.Register(mux)

	transmission := newTransmissionHandler(options, putioClient)
	transmission.Register(mux)

	return loggingMiddleware(mux)
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func wrapResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, status: http.StatusOK}
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
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				log.Println("err:", err, "stack trace:", string(debug.Stack()))
			}
		}()

		start := time.Now()
		wrapped := wrapResponseWriter(w)
		next.ServeHTTP(wrapped, r)
		log.Printf("%d %s [%s] %s",
			wrapped.status,
			r.Method,
			r.URL.EscapedPath(),
			time.Since(start))
	})
}
