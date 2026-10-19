package api

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

const importSGML = "OFXHEADER:100\r\n" +
	"DATA:OFXSGML\r\n" +
	"VERSION:102\r\n" +
	"\r\n" +
	"<OFX><BANKTRANLIST>" +
	"<STMTTRN><TRNTYPE>DEBIT<DTPOSTED>20261001<TRNAMT>-123.45<FITID>NUB-001<NAME>MERCADO SILVA<MEMO>Compra no mercado</STMTTRN>" +
	"<STMTTRN><TRNTYPE>CREDIT<DTPOSTED>20261002120000[-3:BRT]<TRNAMT>50.00<FITID>NUB-002<NAME>SALARIO<MEMO></STMTTRN>" +
	"</BANKTRANLIST></OFX>"

const importXML = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<?OFX OFXHEADER="200" VERSION="211"?>` +
	"<OFX><BANKTRANLIST>" +
	"<STMTTRN><TRNTYPE>DEBIT</TRNTYPE><DTPOSTED>20261001</DTPOSTED><TRNAMT>-123.45</TRNAMT><FITID>C6-001</FITID><NAME>MERCADO SILVA</NAME><MEMO>Compra no mercado</MEMO></STMTTRN>" +
	"</BANKTRANLIST></OFX>"

func (h *harness) uploadOFX(path, filename, content string, accountID int64) *httptest.ResponseRecorder {
	h.t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if accountID != 0 {
		_ = writer.WriteField("account_id", strconv.FormatInt(accountID, 10))
	}
	if filename != "" {
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			h.t.Fatal(err)
		}
		if _, err := io.WriteString(part, content); err != nil {
			h.t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		h.t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, path, body)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

func TestPreviewOFXReturnsOneRowPerTransactionAndWritesNothing(t *testing.T) {
	h := newHarness(t)
	acc, _ := h.accountAndCategory()

	rec := h.uploadOFX("/api/import/ofx/preview", "extrato.ofx", importSGML, acc)
	h.expect(rec, http.StatusOK)
	body := decode[obj](t, rec)
	rows := body["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}

	if left := decode[[]obj](t, h.do(http.MethodGet, "/api/transactions", nil)); len(left) != 0 {
		t.Fatalf("preview wrote %d transactions", len(left))
	}
}

func TestPreviewOFXSuggestsHistoricalCategoryOrOutros(t *testing.T) {
	h := newHarness(t)
	acc, cat := h.accountAndCategory()
	h.create("/api/transactions", obj{
		"type": "expense", "amount_cents": 500, "date": "2026-09-01",
		"account_id": acc, "category_id": cat, "description": "compra no mercado",
	})

	rec := h.uploadOFX("/api/import/ofx/preview", "extrato.ofx", importSGML, acc)
	h.expect(rec, http.StatusOK)
	rows := decode[obj](t, rec)["rows"].([]any)

	row0 := rows[0].(map[string]any)
	if int64(row0["suggested_category_id"].(float64)) != cat {
		t.Fatalf("row0 suggested = %v, want %d (history match)", row0["suggested_category_id"], cat)
	}

	row1 := rows[1].(map[string]any)
	if row1["suggested_category_name"] != "Outros" {
		t.Fatalf("row1 suggested_category_name = %v, want Outros", row1["suggested_category_name"])
	}
}

func TestPreviewOFXFlagsAlreadyImportedRowAsDuplicate(t *testing.T) {
	h := newHarness(t)
	acc, cat := h.accountAndCategory()
	h.expect(h.do(http.MethodPost, "/api/import/ofx/confirm", obj{
		"account_id": acc,
		"rows": []obj{
			{"source_ref": "NUB-001", "date": "2026-10-01", "amount_cents": 12345, "type": "expense", "category_id": cat, "description": "x"},
		},
	}), http.StatusCreated)

	rec := h.uploadOFX("/api/import/ofx/preview", "extrato.ofx", importSGML, acc)
	h.expect(rec, http.StatusOK)
	body := decode[obj](t, rec)
	rows := body["rows"].([]any)
	row0 := rows[0].(map[string]any)
	if row0["is_duplicate"] != true {
		t.Fatalf("row0 = %v, want is_duplicate true", row0)
	}
	summary := body["summary"].(map[string]any)
	if summary["duplicates"].(float64) != 1 {
		t.Fatalf("summary = %v, want 1 duplicate", summary)
	}
}

func TestPreviewOFXRejectsInvalidFile(t *testing.T) {
	h := newHarness(t)
	acc, _ := h.accountAndCategory()
	rec := h.uploadOFX("/api/import/ofx/preview", "extrato.ofx", "not an ofx file", acc)
	h.expectErrorCode(rec, http.StatusBadRequest, "invalid_ofx")
}

func TestPreviewOFXRejectsMissingAccount(t *testing.T) {
	h := newHarness(t)
	rec := h.uploadOFX("/api/import/ofx/preview", "extrato.ofx", importSGML, 9999)
	h.expectErrorCode(rec, http.StatusBadRequest, "validation_error")
}

func TestPreviewOFXRejectsOversizedFile(t *testing.T) {
	h := newHarness(t)
	acc, _ := h.accountAndCategory()
	huge := bytes.Repeat([]byte("a"), 6<<20)
	rec := h.uploadOFX("/api/import/ofx/preview", "extrato.ofx", string(huge), acc)
	h.expect(rec, http.StatusRequestEntityTooLarge)
}

func TestPreviewOFXSGMLAndXMLAreEquivalentForEquivalentContent(t *testing.T) {
	h := newHarness(t)
	acc, _ := h.accountAndCategory()

	sgml := decode[obj](t, h.uploadOFX("/api/import/ofx/preview", "a.ofx", importSGML, acc))
	xml := decode[obj](t, h.uploadOFX("/api/import/ofx/preview", "b.ofx", importXML, acc))
	sgmlRow := sgml["rows"].([]any)[0].(map[string]any)
	xmlRow := xml["rows"].([]any)[0].(map[string]any)
	if sgmlRow["date"] != xmlRow["date"] || sgmlRow["amount_cents"] != xmlRow["amount_cents"] || sgmlRow["type"] != xmlRow["type"] {
		t.Fatalf("sgml = %v, xml = %v", sgmlRow, xmlRow)
	}
}

func TestConfirmOFXCreatesOnlyNonDuplicateRows(t *testing.T) {
	h := newHarness(t)
	acc, cat := h.accountAndCategory()

	rec := h.do(http.MethodPost, "/api/import/ofx/confirm", obj{
		"account_id": acc,
		"rows": []obj{
			{"source_ref": "A", "date": "2026-10-01", "amount_cents": 12345, "type": "expense", "category_id": cat, "description": "Mercado"},
			{"source_ref": "B", "date": "2026-10-02", "amount_cents": 5000, "type": "income", "category_id": cat, "description": "Salario"},
		},
	})
	h.expect(rec, http.StatusCreated)
	body := decode[obj](t, rec)
	if body["created"].(float64) != 2 || body["skipped_duplicates"].(float64) != 0 {
		t.Fatalf("confirm response = %v", body)
	}

	left := decode[[]obj](t, h.do(http.MethodGet, "/api/transactions", nil))
	if len(left) != 2 {
		t.Fatalf("got %d transactions, want 2", len(left))
	}
}

func TestConfirmOFXSkipsRowAlreadyImported(t *testing.T) {
	h := newHarness(t)
	acc, cat := h.accountAndCategory()
	h.expect(h.do(http.MethodPost, "/api/import/ofx/confirm", obj{
		"account_id": acc,
		"rows": []obj{
			{"source_ref": "DUP", "date": "2026-09-01", "amount_cents": 100, "type": "expense", "category_id": cat, "description": "x"},
		},
	}), http.StatusCreated)

	rec := h.do(http.MethodPost, "/api/import/ofx/confirm", obj{
		"account_id": acc,
		"rows": []obj{
			{"source_ref": "DUP", "date": "2026-09-01", "amount_cents": 100, "type": "expense", "category_id": cat, "description": "x"},
		},
	})
	h.expect(rec, http.StatusCreated)
	body := decode[obj](t, rec)
	if body["created"].(float64) != 0 || body["skipped_duplicates"].(float64) != 1 {
		t.Fatalf("confirm response = %v", body)
	}
}

func TestConfirmOFXReimportingSameFileCreatesZero(t *testing.T) {
	h := newHarness(t)
	acc, cat := h.accountAndCategory()
	rows := []obj{
		{"source_ref": "A", "date": "2026-10-01", "amount_cents": 100, "type": "expense", "category_id": cat, "description": "x"},
		{"source_ref": "B", "date": "2026-10-02", "amount_cents": 200, "type": "income", "category_id": cat, "description": "y"},
	}
	h.expect(h.do(http.MethodPost, "/api/import/ofx/confirm", obj{"account_id": acc, "rows": rows}), http.StatusCreated)

	rec := h.do(http.MethodPost, "/api/import/ofx/confirm", obj{"account_id": acc, "rows": rows})
	h.expect(rec, http.StatusCreated)
	body := decode[obj](t, rec)
	if body["created"].(float64) != 0 || body["skipped_duplicates"].(float64) != 2 {
		t.Fatalf("second confirm = %v, want created=0 skipped=2", body)
	}
}

func TestConfirmOFXRejectsInvalidRowAndWritesNothing(t *testing.T) {
	h := newHarness(t)
	acc, cat := h.accountAndCategory()

	rec := h.do(http.MethodPost, "/api/import/ofx/confirm", obj{
		"account_id": acc,
		"rows": []obj{
			{"source_ref": "A", "date": "2026-10-01", "amount_cents": 100, "type": "expense", "category_id": cat, "description": "x"},
			{"source_ref": "B", "date": "2026-10-02", "amount_cents": 0, "type": "expense", "category_id": cat, "description": "y"},
		},
	})
	h.expectErrorCode(rec, http.StatusBadRequest, "validation_error")

	left := decode[[]obj](t, h.do(http.MethodGet, "/api/transactions", nil))
	if len(left) != 0 {
		t.Fatalf("invalid confirm wrote %d transactions", len(left))
	}
}

func TestCreateTransactionIgnoresClientSuppliedSourceRef(t *testing.T) {
	h := newHarness(t)
	acc, cat := h.accountAndCategory()

	rec := h.do(http.MethodPost, "/api/transactions", obj{
		"type": "expense", "amount_cents": 100, "date": "2026-10-01",
		"account_id": acc, "category_id": cat, "description": "x", "source_ref": "STOLEN-FITID",
	})
	h.expect(rec, http.StatusCreated)
	created := decode[obj](t, rec)
	if created["source_ref"] != nil && created["source_ref"] != "" {
		t.Fatalf("created transaction kept a client-supplied source_ref: %v", created["source_ref"])
	}

	preview := h.uploadOFX("/api/import/ofx/preview", "extrato.ofx", importSGML, acc)
	h.expect(preview, http.StatusOK)
	rows := decode[obj](t, preview)["rows"].([]any)
	row0 := rows[0].(map[string]any)
	if row0["is_duplicate"] == true {
		t.Fatalf("manual transaction with empty source_ref must not collide with FITID NUB-001: %v", row0)
	}
}

func TestPreviewOFXTruncatesDescriptionLongerThanTheFieldLimit(t *testing.T) {
	h := newHarness(t)
	acc, cat := h.accountAndCategory()

	longMemo := strings.Repeat("A", 255)
	sgml := "<OFX><BANKTRANLIST>" +
		"<STMTTRN><DTPOSTED>20261001<TRNAMT>-10<FITID>LONG-1<NAME>X<MEMO>" + longMemo + "</STMTTRN>" +
		"</BANKTRANLIST></OFX>"

	preview := h.uploadOFX("/api/import/ofx/preview", "extrato.ofx", sgml, acc)
	h.expect(preview, http.StatusOK)
	row0 := decode[obj](t, preview)["rows"].([]any)[0].(map[string]any)
	description := row0["description"].(string)
	if len([]rune(description)) > 200 {
		t.Fatalf("description has %d runes, want <= 200", len([]rune(description)))
	}

	rec := h.do(http.MethodPost, "/api/import/ofx/confirm", obj{
		"account_id": acc,
		"rows": []obj{
			{"source_ref": "LONG-1", "date": "2026-10-01", "amount_cents": 1000, "type": "expense", "category_id": cat, "description": description},
		},
	})
	h.expect(rec, http.StatusCreated)
}

func TestConfirmOFXRejectsUnknownAccount(t *testing.T) {
	h := newHarness(t)
	_, cat := h.accountAndCategory()
	rec := h.do(http.MethodPost, "/api/import/ofx/confirm", obj{
		"account_id": 9999,
		"rows": []obj{
			{"source_ref": "A", "date": "2026-10-01", "amount_cents": 100, "type": "expense", "category_id": cat, "description": "x"},
		},
	})
	h.expectErrorCode(rec, http.StatusBadRequest, "validation_error")
}
