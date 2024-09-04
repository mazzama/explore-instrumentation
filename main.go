package main

import (
	"context"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	httpRequestTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total Number of HTTP Requests",
		},
		[]string{"path", "status"},
	)
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Histogram of response latency (seconds) of HTTP requests.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"path", "status"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestTotal)
	prometheus.MustRegister(httpRequestDuration)

	//promauto.NewCounter()
}

func main() {
	file, err := os.OpenFile("./logs/app.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		slog.Error("Failed to open logs file: ", err)
		return
	}
	defer file.Close()

	multiWriter := io.MultiWriter(os.Stdout, file)

	var logger = NewLogger(multiWriter)
	slog.SetDefault(logger)

	srv := NewServer(logger)
	go func() {
		if err := srv.ListenAndServe(); err != nil && errors.Is(err, http.ErrServerClosed) {
			slog.Error("ListenAndServe error", slog.String("error", err.Error()))
		}
	}()

	slog.Info("Server started at port :8080")

	gracefulShutdown(srv)
}

func NewServer(logger *slog.Logger) *http.Server {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(prometheusMiddleware)
	r.Use(requestLogger(logger))

	r.Get("/", rootHandler)
	r.Handle("/metrics", promhttp.Handler())

	return &http.Server{
		Addr:     ":8080",
		Handler:  r,
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "Received the request")

	w.Write([]byte("Hello World!"))
}

func gracefulShutdown(srv *http.Server) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

	multiSignalHandler(<-quit)
	slog.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", slog.String("error", err.Error()))
	}

	slog.Info("Server exiting")
}

func multiSignalHandler(signal os.Signal) {
	switch signal {
	case syscall.SIGHUP:
		slog.Info("Signal:", signal.String())
		slog.Info("After hot reload")
	case syscall.SIGINT:
		slog.Info("Signal:", signal.String())
		slog.Info("Interrupt by Ctrl+C")
	case syscall.SIGTERM:
		slog.Info("Signal:", signal.String())
		slog.Info("Process is killed.")
	default:
		slog.Info("Unhandled/unknown signal")
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.status = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

func prometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		path := r.URL.Path

		// Capture the response status
		rw := &responseWriter{w, http.StatusOK}
		next.ServeHTTP(rw, r)
		if path == "/metrics" {
			return
		}

		duration := time.Since(start).Seconds()

		status := rw.status

		httpRequestTotal.WithLabelValues(path, http.StatusText(status)).Inc()
		httpRequestDuration.WithLabelValues(path, http.StatusText(status)).Observe(duration)
	})
}
