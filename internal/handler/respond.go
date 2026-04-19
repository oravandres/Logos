package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/oravandres/Logos/internal/model"
)

type errorBody struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

const maxRequestBodyBytes = 1 << 20

var (
	errRequestBodyEmpty     = errors.New("request body is empty")
	errUnsupportedMediaType = errors.New("content type must be application/json")
	errSingleJSONObjectOnly = errors.New("request body must contain a single JSON object")
)

func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, errorBody{Error: msg})
}

func respondErrorDetail(w http.ResponseWriter, status int, msg, detail string) {
	respondJSON(w, status, errorBody{Error: msg, Details: detail})
}

//nolint:unparam // param name varies across handlers in future PRs
func parseUUID(r *http.Request, param string) (uuid.UUID, error) {
	raw := chi.URLParam(r, param)
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, errors.New("invalid UUID")
	}
	return id, nil
}

func parsePagination(r *http.Request) (limit, offset int32) {
	limit = model.DefaultLimit
	offset = 0

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = int32(min(n, model.MaxLimit))
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = int32(n)
		}
	}
	return
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var maxErr *http.MaxBytesError

	switch {
	case errors.Is(err, errUnsupportedMediaType):
		respondError(w, http.StatusUnsupportedMediaType, err.Error())
	case errors.Is(err, errRequestBodyEmpty):
		respondError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, errSingleJSONObjectOnly):
		respondError(w, http.StatusBadRequest, err.Error())
	case errors.As(err, &maxErr):
		respondError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("request body must be at most %d bytes", maxRequestBodyBytes))
	default:
		respondErrorDetail(w, http.StatusBadRequest, "invalid request body", err.Error())
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) error {
	if r.Body == nil {
		return errRequestBodyEmpty
	}

	if rawContentType := r.Header.Get("Content-Type"); rawContentType != "" {
		mediaType, _, err := mime.ParseMediaType(rawContentType)
		if err != nil || mediaType != "application/json" {
			return errUnsupportedMediaType
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	defer func() { _ = r.Body.Close() }()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return errRequestBodyEmpty
		}
		return err
	}

	var extra any
	if err := dec.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	return errSingleJSONObjectOnly
}
