package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Leonardoongaratto/finance-api/internal/domain"
	"github.com/Leonardoongaratto/finance-api/internal/store"
)

const maxBodyBytes = 1 << 20

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

func writeValidation(w http.ResponseWriter, message string) {
	writeError(w, http.StatusBadRequest, "validation_error", message)
}

func (s *Server) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	return s.decodeWithLimit(w, r, dst, maxBodyBytes)
}

func (s *Server) decodeWithLimit(w http.ResponseWriter, r *http.Request, dst any, limitBytes int64) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json")
		return false
	}

	r.Body = http.MaxBytesReader(w, r.Body, limitBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(dst)
	if err == nil {
		err = expectEOF(decoder)
	}
	if err == nil {
		return true
	}

	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "request body is too large")
	case errors.Is(err, domain.ErrInvalidDate):
		writeValidation(w, domain.ErrInvalidDate.Error())
	default:
		writeValidation(w, "request body is not valid JSON for this resource")
	}
	return false
}

func expectEOF(decoder *json.Decoder) error {
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after JSON body")
	}
	return nil
}

func (s *Server) storeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "resource is in use or already exists")
	case errors.Is(err, store.ErrInvalidReference):
		writeValidation(w, "referenced record does not exist")
	default:
		s.log.Error("store operation failed", "method", r.Method, "route", routePattern(r), "error_kind", errorKind(err))
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
	}
}

// errorKind classifies a failure for the log without its message, which may
// carry stored values.
func errorKind(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "store"
	}
}

func routePattern(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		return rc.RoutePattern()
	}
	return r.URL.Path
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		writeValidation(w, "invalid id in path")
		return 0, false
	}
	return id, true
}

func listHandler[T any](s *Server, list func(context.Context) ([]T, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := list(r.Context())
		if err != nil {
			s.storeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	}
}

func createHandler[T any](s *Server, prepare func(*T) error, create func(context.Context, T) (T, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var item T
		if !s.decode(w, r, &item) {
			return
		}
		if err := prepare(&item); err != nil {
			writeValidation(w, err.Error())
			return
		}
		created, err := create(r.Context(), item)
		if err != nil {
			s.storeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	}
}

func updateHandler[T any](s *Server, setID func(*T, int64), prepare func(*T) error, update func(context.Context, T) (T, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		var item T
		if !s.decode(w, r, &item) {
			return
		}
		setID(&item, id)
		if err := prepare(&item); err != nil {
			writeValidation(w, err.Error())
			return
		}
		updated, err := update(r.Context(), item)
		if err != nil {
			s.storeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

func deleteHandler(s *Server, remove func(context.Context, int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		if err := remove(r.Context(), id); err != nil {
			s.storeError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func childCreateHandler[T any](s *Server, setParent func(*T, int64), prepare func(*T) error, create func(context.Context, T) (T, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parentID, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		var item T
		if !s.decode(w, r, &item) {
			return
		}
		setParent(&item, parentID)
		if err := prepare(&item); err != nil {
			writeValidation(w, err.Error())
			return
		}
		created, err := create(r.Context(), item)
		if errors.Is(err, store.ErrInvalidReference) {
			err = store.ErrNotFound
		}
		if err != nil {
			s.storeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	}
}

func childDeleteHandler(s *Server, remove func(ctx context.Context, parentID, id int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parentID, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		id, ok := pathID(w, r, "childID")
		if !ok {
			return
		}
		if err := remove(r.Context(), parentID, id); err != nil {
			s.storeError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
