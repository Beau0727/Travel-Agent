package http

import (
	"encoding/json"
	stdhttp "net/http"
	"strings"
	"time"

	"zhilv-yuntu-go/internal/application"
	"zhilv-yuntu-go/internal/domain"
	"zhilv-yuntu-go/internal/logging"
)

// Router 是 HTTP 传输层。
// 它只负责协议细节：路由、JSON、状态码、CORS。
// 业务动作都委托给 application usecase。
type Router struct {
	tripUsecase    *application.TripUsecase
	weatherUsecase *application.WeatherUsecase
}

func NewRouter(tripUsecase *application.TripUsecase, weatherUsecase *application.WeatherUsecase) *Router {
	return &Router{
		tripUsecase:    tripUsecase,
		weatherUsecase: weatherUsecase,
	}
}

func (r *Router) Routes() stdhttp.Handler {
	mux := stdhttp.NewServeMux()
	mux.HandleFunc("/", r.handleRoot)
	mux.HandleFunc("/health", r.handleHealth)
	mux.HandleFunc("/trip/generate", r.handleGenerateTrip)
	mux.HandleFunc("/trip/edit", r.handleEditTrip)
	mux.HandleFunc("/trip/save", r.handleSaveTrip)
	mux.HandleFunc("/trip", r.handleTripList)
	mux.HandleFunc("/trip/", r.handleTripDetailOrDelete)
	mux.HandleFunc("/weather/forecast", r.handleWeather)
	mux.HandleFunc("/export/", r.handleExport)
	return requestLogger(cors(mux))
}

func (r *Router) handleRoot(w stdhttp.ResponseWriter, req *stdhttp.Request) {
	if req.URL.Path != "/" {
		writeError(w, req, stdhttp.StatusNotFound, "not found")
		return
	}
	writeJSON(w, stdhttp.StatusOK, map[string]string{"message": "Go Clean Architecture backend is running."})
}

func (r *Router) handleHealth(w stdhttp.ResponseWriter, req *stdhttp.Request) {
	writeJSON(w, stdhttp.StatusOK, map[string]string{"status": "ok"})
}

func (r *Router) handleGenerateTrip(w stdhttp.ResponseWriter, req *stdhttp.Request) {
	if req.Method != stdhttp.MethodPost {
		writeError(w, req, stdhttp.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload domain.TripRequest
	if !decodeJSON(w, req, &payload) {
		return
	}
	logging.Info(req.Context(), "trip generation request decoded",
		"destination", payload.Destination,
		"start_date", payload.StartDate,
		"end_date", payload.EndDate,
		"travelers", payload.Travelers,
		"preferences", len(payload.Preferences),
	)
	itinerary, err := r.tripUsecase.Generate(req.Context(), payload)
	if err != nil {
		writeError(w, req, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stdhttp.StatusOK, itinerary)
}

func (r *Router) handleEditTrip(w stdhttp.ResponseWriter, req *stdhttp.Request) {
	if req.Method != stdhttp.MethodPost {
		writeError(w, req, stdhttp.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload domain.TripEditRequest
	if !decodeJSON(w, req, &payload) {
		return
	}
	logging.Info(req.Context(), "trip edit request decoded",
		"trip_id", payload.TripID,
		"edit_scope", payload.EditScope,
		"current_days", len(payload.CurrentItinerary.Days),
	)
	itinerary, err := r.tripUsecase.Edit(req.Context(), payload)
	if err != nil {
		writeError(w, req, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stdhttp.StatusOK, itinerary)
}

func (r *Router) handleSaveTrip(w stdhttp.ResponseWriter, req *stdhttp.Request) {
	if req.Method != stdhttp.MethodPost {
		writeError(w, req, stdhttp.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload domain.TripSaveRequest
	if !decodeJSON(w, req, &payload) {
		return
	}
	logging.Info(req.Context(), "trip save request decoded",
		"trip_id", payload.Itinerary.TripID,
		"destination", payload.Itinerary.Destination,
		"days", len(payload.Itinerary.Days),
	)
	tripID, err := r.tripUsecase.Save(req.Context(), payload)
	if err != nil {
		writeError(w, req, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stdhttp.StatusOK, map[string]string{
		"message": "Trip itinerary saved successfully.",
		"trip_id": tripID,
	})
}

func (r *Router) handleTripList(w stdhttp.ResponseWriter, req *stdhttp.Request) {
	if req.Method != stdhttp.MethodGet {
		writeError(w, req, stdhttp.StatusMethodNotAllowed, "method not allowed")
		return
	}
	response, err := r.tripUsecase.List(req.Context())
	if err != nil {
		writeError(w, req, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stdhttp.StatusOK, response)
}

func (r *Router) handleTripDetailOrDelete(w stdhttp.ResponseWriter, req *stdhttp.Request) {
	tripID := strings.TrimPrefix(req.URL.Path, "/trip/")
	if tripID == "" {
		writeError(w, req, stdhttp.StatusNotFound, "trip id required")
		return
	}
	switch req.Method {
	case stdhttp.MethodGet:
		detail, ok, err := r.tripUsecase.Get(req.Context(), tripID)
		if err != nil {
			writeError(w, req, stdhttp.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			writeError(w, req, stdhttp.StatusNotFound, "Trip not found.")
			return
		}
		writeJSON(w, stdhttp.StatusOK, detail)
	case stdhttp.MethodDelete:
		deleted, err := r.tripUsecase.Delete(req.Context(), tripID)
		if err != nil {
			writeError(w, req, stdhttp.StatusInternalServerError, err.Error())
			return
		}
		if !deleted {
			writeError(w, req, stdhttp.StatusNotFound, "Trip not found.")
			return
		}
		writeJSON(w, stdhttp.StatusOK, map[string]string{"message": "Trip itinerary deleted successfully.", "trip_id": tripID})
	default:
		writeError(w, req, stdhttp.StatusMethodNotAllowed, "method not allowed")
	}
}

func (r *Router) handleWeather(w stdhttp.ResponseWriter, req *stdhttp.Request) {
	city := req.URL.Query().Get("city")
	if city == "" {
		writeError(w, req, stdhttp.StatusBadRequest, "city is required")
		return
	}
	writeJSON(w, stdhttp.StatusOK, r.weatherUsecase.Forecast(req.Context(), city))
}

func (r *Router) handleExport(w stdhttp.ResponseWriter, req *stdhttp.Request) {
	parts := strings.Split(strings.TrimPrefix(req.URL.Path, "/export/"), "/")
	if len(parts) != 2 {
		writeError(w, req, stdhttp.StatusNotFound, "not found")
		return
	}
	tripID, format := parts[0], parts[1]
	if format != "markdown" {
		writeError(w, req, stdhttp.StatusNotImplemented, "Go backend currently implements markdown export only.")
		return
	}
	markdown, ok, err := r.tripUsecase.ExportMarkdown(req.Context(), tripID)
	if err != nil {
		writeError(w, req, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, req, stdhttp.StatusNotFound, "Trip not found.")
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write([]byte(markdown))
}

func decodeJSON(w stdhttp.ResponseWriter, req *stdhttp.Request, target any) bool {
	defer req.Body.Close()
	if err := json.NewDecoder(req.Body).Decode(target); err != nil {
		writeError(w, req, stdhttp.StatusBadRequest, "invalid json: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w stdhttp.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w stdhttp.ResponseWriter, req *stdhttp.Request, status int, message string) {
	if status >= 500 {
		logging.Error(req.Context(), "http request failed", "status", status, "error", message)
	} else {
		logging.Warn(req.Context(), "http request rejected", "status", status, "error", message)
	}
	writeJSON(w, status, map[string]string{"detail": message})
}

type loggingResponseWriter struct {
	stdhttp.ResponseWriter
	status int
	bytes  int
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(stdhttp.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}

func requestLogger(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, req *stdhttp.Request) {
		requestID := req.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = logging.NewRequestID()
		}
		ctx := logging.WithRequestID(req.Context(), requestID)
		req = req.WithContext(ctx)
		w.Header().Set("X-Request-ID", requestID)

		start := time.Now()
		logging.Info(ctx, "http request started",
			"method", req.Method,
			"path", req.URL.Path,
			"query", req.URL.RawQuery,
			"remote_addr", req.RemoteAddr,
		)

		recorder := &loggingResponseWriter{ResponseWriter: w}
		next.ServeHTTP(recorder, req)
		status := recorder.status
		if status == 0 {
			status = stdhttp.StatusOK
		}

		logArgs := []any{
			"method", req.Method,
			"path", req.URL.Path,
			"status", status,
			"bytes", recorder.bytes,
			"duration_ms", time.Since(start).Milliseconds(),
		}
		if status >= 500 {
			logging.Error(ctx, "http request completed", logArgs...)
			return
		}
		logging.Info(ctx, "http request completed", logArgs...)
	})
}

func cors(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, req *stdhttp.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if req.Method == stdhttp.MethodOptions {
			w.WriteHeader(stdhttp.StatusNoContent)
			return
		}
		next.ServeHTTP(w, req)
	})
}
