package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Leonardoongaratto/finance-api/internal/domain"
)

const (
	maxImportBodyBytes = 5 << 20
	categoryOutros     = "Outros"
)

type ofxPreviewRow struct {
	SourceRef             string                 `json:"source_ref"`
	Date                  domain.Date            `json:"date"`
	AmountCents           int64                  `json:"amount_cents"`
	Type                  domain.TransactionType `json:"type"`
	Description           string                 `json:"description"`
	SuggestedCategoryID   int64                  `json:"suggested_category_id"`
	SuggestedCategoryName string                 `json:"suggested_category_name"`
	IsDuplicate           bool                   `json:"is_duplicate"`
}

type ofxPreviewSummary struct {
	Total      int `json:"total"`
	Duplicates int `json:"duplicates"`
}

type ofxPreviewResponse struct {
	AccountID int64             `json:"account_id"`
	Rows      []ofxPreviewRow   `json:"rows"`
	Summary   ofxPreviewSummary `json:"summary"`
}

type ofxConfirmRow struct {
	SourceRef   string                 `json:"source_ref"`
	Date        domain.Date            `json:"date"`
	AmountCents int64                  `json:"amount_cents"`
	Type        domain.TransactionType `json:"type"`
	CategoryID  int64                  `json:"category_id"`
	Description string                 `json:"description"`
}

type ofxConfirmRequest struct {
	AccountID int64           `json:"account_id"`
	Rows      []ofxConfirmRow `json:"rows"`
}

type ofxConfirmResponse struct {
	Created           int `json:"created"`
	SkippedDuplicates int `json:"skipped_duplicates"`
}

func (s *Server) previewOFXImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBodyBytes)
	if err := r.ParseMultipartForm(maxImportBodyBytes); err != nil {
		s.handleImportBodyError(w, err)
		return
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}

	accountID, err := strconv.ParseInt(r.FormValue("account_id"), 10, 64)
	if err != nil || accountID <= 0 {
		writeValidation(w, "account_id is required and must be a positive integer")
		return
	}
	exists, err := s.accountExists(r.Context(), accountID)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	if !exists {
		writeValidation(w, "account_id does not exist")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeValidation(w, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".ofx") {
		writeValidation(w, "file must have the .ofx extension")
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		s.handleImportBodyError(w, err)
		return
	}

	ofxRows, err := domain.ParseOFX(data)
	s.log.Info("ofx preview parsed",
		"method", r.Method, "route", routePattern(r),
		"file_bytes", len(data), "line_count", bytes.Count(data, []byte("\n"))+1)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_ofx", "o arquivo enviado não é um extrato OFX válido")
		return
	}

	response, err := s.buildPreview(r.Context(), accountID, ofxRows)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleImportBodyError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "request body is too large")
		return
	}
	writeValidation(w, "request body is not a valid multipart/form-data upload")
}

// truncateRunes keeps a bank-supplied description within the field's
// storage limit (MEMO/NAME can be longer in the OFX spec than this app's
// description column) instead of failing the whole confirm.
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func (s *Server) accountExists(ctx context.Context, id int64) (bool, error) {
	accounts, err := s.store.ListAccounts(ctx)
	if err != nil {
		return false, err
	}
	for _, a := range accounts {
		if a.ID == id {
			return true, nil
		}
	}
	return false, nil
}

func (s *Server) otherCategoryID(ctx context.Context) (int64, map[int64]string, error) {
	categories, err := s.store.ListCategories(ctx)
	if err != nil {
		return 0, nil, err
	}
	names := make(map[int64]string, len(categories))
	var fallback int64
	for _, c := range categories {
		names[c.ID] = c.Name
		if c.Name == categoryOutros {
			fallback = c.ID
		}
	}
	return fallback, names, nil
}

func (s *Server) buildPreview(ctx context.Context, accountID int64, ofxRows []domain.OFXTransaction) (ofxPreviewResponse, error) {
	refs := make([]string, len(ofxRows))
	for i, row := range ofxRows {
		refs[i] = row.FITID
	}
	existing, err := s.store.DuplicateSourceRefs(ctx, accountID, refs)
	if err != nil {
		return ofxPreviewResponse{}, err
	}
	history, err := s.store.LastCategoryByDescription(ctx, accountID)
	if err != nil {
		return ofxPreviewResponse{}, err
	}
	fallbackID, names, err := s.otherCategoryID(ctx)
	if err != nil {
		return ofxPreviewResponse{}, err
	}

	response := ofxPreviewResponse{AccountID: accountID, Rows: make([]ofxPreviewRow, len(ofxRows))}
	seen := map[string]bool{}
	for i, o := range ofxRows {
		description := truncateRunes(o.Description, maxTextLength)
		normalized := domain.NormalizeDescription(description)
		categoryID := domain.SuggestCategory(history, normalized, fallbackID)
		duplicate := existing[o.FITID] || seen[o.FITID]
		seen[o.FITID] = true
		if duplicate {
			response.Summary.Duplicates++
		}
		response.Rows[i] = ofxPreviewRow{
			SourceRef:             o.FITID,
			Date:                  o.Date,
			AmountCents:           o.AmountCents,
			Type:                  o.Type,
			Description:           description,
			SuggestedCategoryID:   categoryID,
			SuggestedCategoryName: names[categoryID],
			IsDuplicate:           duplicate,
		}
	}
	response.Summary.Total = len(ofxRows)
	return response, nil
}

func (s *Server) confirmOFXImport(w http.ResponseWriter, r *http.Request) {
	var req ofxConfirmRequest
	if !s.decodeWithLimit(w, r, &req, maxImportBodyBytes) {
		return
	}
	if req.AccountID <= 0 {
		writeValidation(w, "account_id is required")
		return
	}
	exists, err := s.accountExists(r.Context(), req.AccountID)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	if !exists {
		writeValidation(w, "account_id does not exist")
		return
	}

	transactions := make([]domain.Transaction, len(req.Rows))
	for i, row := range req.Rows {
		t := domain.Transaction{
			Type:        row.Type,
			AmountCents: row.AmountCents,
			Date:        row.Date,
			AccountID:   req.AccountID,
			CategoryID:  row.CategoryID,
			Description: row.Description,
			SourceRef:   strings.TrimSpace(row.SourceRef),
		}
		if err := validateTransaction(&t); err != nil {
			writeValidation(w, err.Error())
			return
		}
		transactions[i] = t
	}

	created, skipped, err := s.store.ImportTransactions(r.Context(), transactions)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	s.log.Info("ofx confirm",
		"method", r.Method, "route", routePattern(r),
		"row_count", len(req.Rows), "created", len(created), "skipped_duplicates", skipped)
	writeJSON(w, http.StatusCreated, ofxConfirmResponse{Created: len(created), SkippedDuplicates: skipped})
}
