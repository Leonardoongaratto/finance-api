package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Leonardoongaratto/finance-api/internal/store"
)

var fixedNow = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

type harness struct {
	t       *testing.T
	handler http.Handler
	logs    *bytes.Buffer
}

const testOrigin = "http://127.0.0.1:5173"

func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessWithOrigins(t, []string{testOrigin, "http://localhost:4173"})
}

func newHarnessWithOrigins(t *testing.T, origins []string) *harness {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	logs := &bytes.Buffer{}
	handler := New(Config{
		Store:       db,
		Now:         func() time.Time { return fixedNow },
		Logger:      slog.New(slog.NewTextHandler(logs, nil)),
		CORSOrigins: origins,
	})
	return &harness{t: t, handler: handler, logs: logs}
}

func (h *harness) do(method, path string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			h.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Host = "127.0.0.1:8080"
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

type obj = map[string]any

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func (h *harness) expect(rec *httptest.ResponseRecorder, status int) {
	h.t.Helper()
	if rec.Code != status {
		h.t.Fatalf("status = %d, want %d; body = %s", rec.Code, status, rec.Body.String())
	}
}

func (h *harness) create(path string, body any) obj {
	h.t.Helper()
	rec := h.do(http.MethodPost, path, body)
	h.expect(rec, http.StatusCreated)
	return decode[obj](h.t, rec)
}

func id(o obj) string {
	return strconv.FormatInt(int64(o["id"].(float64)), 10)
}

func (h *harness) accountAndCategory() (accountID, categoryID int64) {
	h.t.Helper()
	acc := h.create("/api/accounts", obj{"name": "Conta", "type": "checking"})
	cats := decode[[]obj](h.t, h.do(http.MethodGet, "/api/categories", nil))
	return int64(acc["id"].(float64)), int64(cats[0]["id"].(float64))
}

func (h *harness) expectErrorCode(rec *httptest.ResponseRecorder, status int, code string) {
	h.t.Helper()
	h.expect(rec, status)
	body := decode[struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}](h.t, rec)
	if body.Error.Code != code || body.Error.Message == "" {
		h.t.Fatalf("error body = %s, want code %s", rec.Body.String(), code)
	}
}

func TestHealth(t *testing.T) {
	h := newHarness(t)
	rec := h.do(http.MethodGet, "/health", nil)
	h.expect(rec, http.StatusOK)
	if decode[obj](t, rec)["status"] != "ok" {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestInstallmentPurchaseCreatesLinkedMonthlyTransactions(t *testing.T) {
	h := newHarness(t)
	acc, cat := h.accountAndCategory()

	rec := h.do(http.MethodPost, "/api/transactions/installments", obj{
		"total_cents": 10000, "installments": 3, "first_date": "2026-10-01",
		"account_id": acc, "category_id": cat, "description": "TV",
	})
	h.expect(rec, http.StatusCreated)
	txs := decode[[]obj](t, rec)
	if len(txs) != 3 {
		t.Fatalf("got %d installments", len(txs))
	}
	wantAmounts := []float64{3334, 3333, 3333}
	var sum float64
	group := txs[0]["installment_group"]
	for i, tx := range txs {
		sum += tx["amount_cents"].(float64)
		if tx["amount_cents"] != wantAmounts[i] || !strings.Contains(tx["description"].(string), strconv.Itoa(i+1)+"/3") || tx["installment_group"] != group || group == "" {
			t.Errorf("installment %d = %v", i, tx)
		}
	}
	if sum != 10000 {
		t.Errorf("sum = %v", sum)
	}
	if txs[1]["date"] != "2026-11-01" {
		t.Errorf("second installment date = %v", txs[1]["date"])
	}
}

func TestDeleteInstallmentGroupRemovesAllInstallments(t *testing.T) {
	h := newHarness(t)
	acc, cat := h.accountAndCategory()
	rec := h.do(http.MethodPost, "/api/transactions/installments", obj{
		"total_cents": 9000, "installments": 3, "first_date": "2026-10-01", "account_id": acc, "category_id": cat, "description": "x",
	})
	group := decode[[]obj](t, rec)[0]["installment_group"].(string)

	h.expect(h.do(http.MethodDelete, "/api/installments/"+group, nil), http.StatusNoContent)
	if left := decode[[]obj](t, h.do(http.MethodGet, "/api/transactions", nil)); len(left) != 0 {
		t.Fatalf("%d transactions left", len(left))
	}
	h.expectErrorCode(h.do(http.MethodDelete, "/api/installments/"+group, nil), http.StatusNotFound, "not_found")
}

func TestInstallmentValidation(t *testing.T) {
	h := newHarness(t)
	acc, cat := h.accountAndCategory()
	base := func(mut func(obj)) obj {
		o := obj{"total_cents": 10000, "installments": 3, "first_date": "2026-10-01", "account_id": acc, "category_id": cat, "description": "x"}
		mut(o)
		return o
	}
	for _, body := range []obj{
		base(func(o obj) { o["installments"] = 1 }),
		base(func(o obj) { o["installments"] = 49 }),
		base(func(o obj) { o["total_cents"] = 0 }),
		base(func(o obj) { o["total_cents"] = 2 }),
		base(func(o obj) { o["first_date"] = "2026-02-31" }),
		base(func(o obj) { delete(o, "account_id") }),
		base(func(o obj) { o["category_id"] = 9999 }),
	} {
		h.expectErrorCode(h.do(http.MethodPost, "/api/transactions/installments", body), http.StatusBadRequest, "validation_error")
	}
	if left := decode[[]obj](t, h.do(http.MethodGet, "/api/transactions", nil)); len(left) != 0 {
		t.Fatalf("invalid installments wrote %d rows", len(left))
	}
}

func TestInvalidBodiesAreRejectedAndNothingIsWritten(t *testing.T) {
	h := newHarness(t)
	acc, cat := h.accountAndCategory()
	valid := func(mut func(obj)) obj {
		o := obj{"type": "expense", "amount_cents": 1500, "date": "2026-10-01", "account_id": acc, "category_id": cat, "description": "x"}
		mut(o)
		return o
	}
	bodies := []any{
		valid(func(o obj) { o["amount_cents"] = 0 }),
		valid(func(o obj) { o["amount_cents"] = -5 }),
		valid(func(o obj) { o["amount_cents"] = 1.5 }),
		valid(func(o obj) { o["amount_cents"] = 100000000000000 }),
		valid(func(o obj) { o["date"] = "2026-13-40" }),
		valid(func(o obj) { delete(o, "date") }),
		valid(func(o obj) { delete(o, "amount_cents") }),
		valid(func(o obj) { o["type"] = "transfer" }),
		valid(func(o obj) { o["account_id"] = 9999 }),
		valid(func(o obj) { o["unexpected"] = true }),
		valid(func(o obj) { o["description"] = strings.Repeat("a", 500) }),
		`{"type": "expense", `,
		`[]`,
		``,
	}
	for i, body := range bodies {
		rec := h.do(http.MethodPost, "/api/transactions", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %d: status %d (%s)", i, rec.Code, rec.Body.String())
		}
	}
	if left := decode[[]obj](t, h.do(http.MethodGet, "/api/transactions", nil)); len(left) != 0 {
		t.Fatalf("invalid bodies wrote %d rows", len(left))
	}
}

func TestInvalidBodiesForOtherResources(t *testing.T) {
	h := newHarness(t)
	cases := map[string]obj{
		"/api/accounts":    {"name": "", "type": "checking"},
		"/api/categories":  {"name": "   "},
		"/api/investments": {"name": "CDB", "type": "lottery"},
		"/api/goals":       {"name": "Meta", "target_cents": 0, "deadline": "2027-01-01"},
		"/api/debts":       {"name": "Divida", "original_cents": 1000, "monthly_rate_bp": -1, "start_date": "2026-01-01"},
	}
	for path, body := range cases {
		h.expectErrorCode(h.do(http.MethodPost, path, body), http.StatusBadRequest, "validation_error")
	}
	h.expectErrorCode(h.do(http.MethodPost, "/api/accounts", obj{"name": "x", "type": "wallet"}), http.StatusBadRequest, "validation_error")
	h.expectErrorCode(h.do(http.MethodPost, "/api/goals", obj{"name": "Meta", "target_cents": 100, "deadline": "2026-01-01", "start_date": "2026-06-01"}), http.StatusBadRequest, "validation_error")
	h.expectErrorCode(h.do(http.MethodPost, "/api/debts", obj{"name": "D", "original_cents": 100, "monthly_rate_bp": 20000, "start_date": "2026-01-01"}), http.StatusBadRequest, "validation_error")
	h.expectErrorCode(h.do(http.MethodPost, "/api/debts", obj{"name": "D", "original_cents": 100, "monthly_payment_cents": -1, "start_date": "2026-01-01"}), http.StatusBadRequest, "validation_error")
	for _, path := range []string{"/api/accounts", "/api/investments", "/api/goals", "/api/debts"} {
		if list := decode[[]obj](t, h.do(http.MethodGet, path, nil)); len(list) != 0 {
			t.Errorf("%s wrote rows: %v", path, list)
		}
	}
}

func TestChildResourceValidation(t *testing.T) {
	h := newHarness(t)
	inv := h.create("/api/investments", obj{"name": "CDB", "type": "fixed_income"})
	goal := h.create("/api/goals", obj{"name": "Meta", "target_cents": 1000, "deadline": "2027-01-01"})
	debt := h.create("/api/debts", obj{"name": "D", "original_cents": 1000, "start_date": "2026-01-01"})

	bad := obj{"date": "2026-01-01", "amount_cents": 0}
	h.expectErrorCode(h.do(http.MethodPost, "/api/investments/"+id(inv)+"/contributions", bad), http.StatusBadRequest, "validation_error")
	h.expectErrorCode(h.do(http.MethodPost, "/api/goals/"+id(goal)+"/contributions", bad), http.StatusBadRequest, "validation_error")
	h.expectErrorCode(h.do(http.MethodPost, "/api/debts/"+id(debt)+"/payments", bad), http.StatusBadRequest, "validation_error")
	h.expectErrorCode(h.do(http.MethodPost, "/api/investments/"+id(inv)+"/valuations", obj{"date": "2026-01-01", "value_cents": -1}), http.StatusBadRequest, "validation_error")
	h.expect(h.do(http.MethodPost, "/api/investments/"+id(inv)+"/valuations", obj{"date": "2026-01-01", "value_cents": 0}), http.StatusCreated)
}

func TestUnknownIDsReturnNotFound(t *testing.T) {
	h := newHarness(t)
	acc, cat := h.accountAndCategory()
	tx := obj{"type": "income", "amount_cents": 100, "date": "2026-10-01", "account_id": acc, "category_id": cat}

	cases := []struct {
		method, path string
		body         any
	}{
		{http.MethodPut, "/api/accounts/999", obj{"name": "x", "type": "cash"}},
		{http.MethodDelete, "/api/accounts/999", nil},
		{http.MethodPut, "/api/categories/999", obj{"name": "x"}},
		{http.MethodDelete, "/api/categories/999", nil},
		{http.MethodPut, "/api/transactions/999", tx},
		{http.MethodDelete, "/api/transactions/999", nil},
		{http.MethodPut, "/api/investments/999", obj{"name": "x", "type": "other"}},
		{http.MethodDelete, "/api/investments/999", nil},
		{http.MethodPost, "/api/investments/999/contributions", obj{"date": "2026-01-01", "amount_cents": 1}},
		{http.MethodPost, "/api/investments/999/valuations", obj{"date": "2026-01-01", "value_cents": 1}},
		{http.MethodDelete, "/api/investments/999/contributions/1", nil},
		{http.MethodDelete, "/api/investments/999/valuations/1", nil},
		{http.MethodPut, "/api/goals/999", obj{"name": "x", "target_cents": 5, "deadline": "2027-01-01", "start_date": "2026-01-01"}},
		{http.MethodDelete, "/api/goals/999", nil},
		{http.MethodPost, "/api/goals/999/contributions", obj{"date": "2026-01-01", "amount_cents": 1}},
		{http.MethodDelete, "/api/goals/999/contributions/1", nil},
		{http.MethodPut, "/api/debts/999", obj{"name": "x", "original_cents": 5, "start_date": "2026-01-01"}},
		{http.MethodDelete, "/api/debts/999", nil},
		{http.MethodPost, "/api/debts/999/payments", obj{"date": "2026-01-01", "amount_cents": 1}},
		{http.MethodDelete, "/api/debts/999/payments/1", nil},
	}
	for _, c := range cases {
		h.expectErrorCode(h.do(c.method, c.path, c.body), http.StatusNotFound, "not_found")
	}
}

func TestMalformedIDsAreValidationErrors(t *testing.T) {
	h := newHarness(t)
	h.expectErrorCode(h.do(http.MethodDelete, "/api/accounts/abc", nil), http.StatusBadRequest, "validation_error")
	h.expectErrorCode(h.do(http.MethodDelete, "/api/debts/1/payments/xyz", nil), http.StatusBadRequest, "validation_error")
	h.expectErrorCode(h.do(http.MethodDelete, "/api/accounts/-4", nil), http.StatusBadRequest, "validation_error")
}

func TestDeletingCategoryInUseIsConflict(t *testing.T) {
	h := newHarness(t)
	acc, cat := h.accountAndCategory()
	h.create("/api/transactions", obj{"type": "expense", "amount_cents": 100, "date": "2026-10-01", "account_id": acc, "category_id": cat})

	h.expectErrorCode(h.do(http.MethodDelete, "/api/categories/"+strconv.FormatInt(cat, 10), nil), http.StatusConflict, "conflict")
	h.expectErrorCode(h.do(http.MethodDelete, "/api/accounts/"+strconv.FormatInt(acc, 10), nil), http.StatusConflict, "conflict")
	h.expectErrorCode(h.do(http.MethodPost, "/api/categories", obj{"name": "Moradia"}), http.StatusConflict, "conflict")
}

func TestCategoryAndAccountCRUD(t *testing.T) {
	h := newHarness(t)
	cats := decode[[]obj](t, h.do(http.MethodGet, "/api/categories", nil))
	if len(cats) != 9 {
		t.Fatalf("seeded categories = %d", len(cats))
	}
	created := h.create("/api/categories", obj{"name": "Pets"})
	rec := h.do(http.MethodPut, "/api/categories/"+id(created), obj{"name": "Animais"})
	h.expect(rec, http.StatusOK)
	if decode[obj](t, rec)["name"] != "Animais" {
		t.Fatalf("body = %s", rec.Body.String())
	}
	h.expect(h.do(http.MethodDelete, "/api/categories/"+id(created), nil), http.StatusNoContent)

	acc := h.create("/api/accounts", obj{"name": "Nubank", "type": "credit_card"})
	rec = h.do(http.MethodPut, "/api/accounts/"+id(acc), obj{"name": "Nubank Ultravioleta", "type": "credit_card"})
	h.expect(rec, http.StatusOK)
	h.expect(h.do(http.MethodDelete, "/api/accounts/"+id(acc), nil), http.StatusNoContent)
}

func TestTransactionCRUDAndFilters(t *testing.T) {
	h := newHarness(t)
	acc, cat := h.accountAndCategory()
	first := h.create("/api/transactions", obj{"type": "income", "amount_cents": 90000, "date": "2026-09-05", "account_id": acc, "category_id": cat, "description": "Salario"})
	h.create("/api/transactions", obj{"type": "expense", "amount_cents": 1500, "date": "2026-10-01", "account_id": acc, "category_id": cat})

	count := func(query string) int {
		rec := h.do(http.MethodGet, "/api/transactions"+query, nil)
		h.expect(rec, http.StatusOK)
		return len(decode[[]obj](t, rec))
	}
	if count("") != 2 || count("?type=income") != 1 || count("?from=2026-10-01") != 1 || count("?to=2026-09-30") != 1 ||
		count("?account_id="+strconv.FormatInt(acc, 10)) != 2 || count("?category_id="+strconv.FormatInt(cat+1, 10)) != 0 {
		t.Fatal("filters returned unexpected counts")
	}
	for _, bad := range []string{"?type=x", "?from=oops", "?to=2026-99-99", "?account_id=abc", "?category_id=-1"} {
		h.expectErrorCode(h.do(http.MethodGet, "/api/transactions"+bad, nil), http.StatusBadRequest, "validation_error")
	}

	rec := h.do(http.MethodPut, "/api/transactions/"+id(first), obj{"type": "income", "amount_cents": 95000, "date": "2026-09-05", "account_id": acc, "category_id": cat, "description": "Salario"})
	h.expect(rec, http.StatusOK)
	if decode[obj](t, rec)["amount_cents"] != float64(95000) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	h.expect(h.do(http.MethodDelete, "/api/transactions/"+id(first), nil), http.StatusNoContent)
}

func TestInvestmentListShowsGainAndReturn(t *testing.T) {
	h := newHarness(t)
	inv := h.create("/api/investments", obj{"name": "CDB", "type": "fixed_income"})
	base := "/api/investments/" + id(inv)
	h.create(base+"/contributions", obj{"date": "2026-01-10", "amount_cents": 100000})
	second := h.create(base+"/contributions", obj{"date": "2026-02-10", "amount_cents": 50000})
	valuation := h.create(base+"/valuations", obj{"date": "2026-03-10", "value_cents": 180000})

	list := decode[[]obj](t, h.do(http.MethodGet, "/api/investments", nil))
	if len(list) != 1 {
		t.Fatalf("list = %v", list)
	}
	got := list[0]
	if got["invested_cents"] != float64(150000) || got["current_cents"] != float64(180000) || got["gain_cents"] != float64(30000) || got["return_pct"] != float64(20) {
		t.Fatalf("investment = %v", got)
	}
	if len(got["contributions"].([]any)) != 2 || len(got["valuations"].([]any)) != 1 {
		t.Fatalf("children = %v", got)
	}

	h.expect(h.do(http.MethodPut, "/api/investments/"+id(inv), obj{"name": "CDB 120%", "type": "fixed_income"}), http.StatusOK)
	h.expect(h.do(http.MethodDelete, base+"/contributions/"+id(second), nil), http.StatusNoContent)
	h.expect(h.do(http.MethodDelete, base+"/valuations/"+id(valuation), nil), http.StatusNoContent)
	h.expect(h.do(http.MethodDelete, base, nil), http.StatusNoContent)
}

func TestGoalListShowsProgressAndMonthlyNeeded(t *testing.T) {
	h := newHarness(t)
	goal := h.create("/api/goals", obj{"name": "Viagem", "target_cents": 1200000, "deadline": "2027-07-02", "start_date": "2026-10-01"})
	base := "/api/goals/" + id(goal)
	contribution := h.create(base+"/contributions", obj{"date": "2026-10-02", "amount_cents": 300000})

	got := decode[[]obj](t, h.do(http.MethodGet, "/api/goals", nil))[0]
	if got["progress_pct"] != float64(25) || got["remaining_cents"] != float64(900000) || got["monthly_needed_cents"] != float64(100000) || got["saved_cents"] != float64(300000) || got["months_left"] != float64(9) {
		t.Fatalf("goal = %v", got)
	}
	if status := got["status"]; status != "on_track" && status != "behind" {
		t.Fatalf("status = %v", status)
	}

	h.expect(h.do(http.MethodPut, "/api/goals/"+id(goal), obj{"name": "Viagem", "target_cents": 1300000, "deadline": "2027-07-02", "start_date": "2026-10-01"}), http.StatusOK)
	h.expect(h.do(http.MethodDelete, base+"/contributions/"+id(contribution), nil), http.StatusNoContent)
	h.expect(h.do(http.MethodDelete, base, nil), http.StatusNoContent)
}

func TestGoalStartDateDefaultsToToday(t *testing.T) {
	h := newHarness(t)
	goal := h.create("/api/goals", obj{"name": "Reserva", "target_cents": 1000, "deadline": "2027-01-01"})
	if goal["start_date"] != "2026-10-02" {
		t.Fatalf("goal = %v", goal)
	}
}

func TestGoalOverdueAndDoneStatus(t *testing.T) {
	h := newHarness(t)
	overdue := h.create("/api/goals", obj{"name": "Atrasada", "target_cents": 10000, "deadline": "2026-09-01", "start_date": "2026-01-01"})
	done := h.create("/api/goals", obj{"name": "Feita", "target_cents": 10000, "deadline": "2026-09-01", "start_date": "2026-01-01"})
	h.create("/api/goals/"+id(done)+"/contributions", obj{"date": "2026-05-01", "amount_cents": 10000})

	statuses := map[string]any{}
	for _, g := range decode[[]obj](t, h.do(http.MethodGet, "/api/goals", nil)) {
		statuses[g["name"].(string)] = g["status"]
	}
	if statuses["Atrasada"] != "overdue" || statuses["Feita"] != "done" {
		t.Fatalf("statuses = %v (overdue id %s)", statuses, id(overdue))
	}
}

func TestDebtProjectionPaysOffInFourMonths(t *testing.T) {
	h := newHarness(t)
	h.create("/api/debts", obj{"name": "Emprestimo", "creditor": "Banco", "original_cents": 1000000, "monthly_rate_bp": 0, "monthly_payment_cents": 250000, "start_date": "2026-10-01"})

	debt := decode[[]obj](t, h.do(http.MethodGet, "/api/debts", nil))[0]
	if debt["payoff_months"] != float64(4) || debt["payoff_month"] != "2027-02" || debt["never_pays_off"] != false || debt["balance_cents"] != float64(1000000) {
		t.Fatalf("debt = %v", debt)
	}
}

func TestDebtBalanceAppliesInterestAndPaymentsNeverNegative(t *testing.T) {
	h := newHarness(t)
	debt := h.create("/api/debts", obj{"name": "Cartao", "original_cents": 10000, "monthly_rate_bp": 100, "monthly_payment_cents": 1000, "start_date": "2026-08-01"})
	base := "/api/debts/" + id(debt)
	h.create(base+"/payments", obj{"date": "2026-08-20", "amount_cents": 1000})
	pay := h.create(base+"/payments", obj{"date": "2026-09-20", "amount_cents": 1000})

	got := decode[[]obj](t, h.do(http.MethodGet, "/api/debts", nil))[0]
	if got["balance_cents"] != float64(8171) {
		t.Fatalf("balance = %v", got["balance_cents"])
	}
	if len(got["payments"].([]any)) != 2 {
		t.Fatalf("payments = %v", got["payments"])
	}

	h.create(base+"/payments", obj{"date": "2026-10-05", "amount_cents": 99999999})
	got = decode[[]obj](t, h.do(http.MethodGet, "/api/debts", nil))[0]
	if got["balance_cents"] != float64(0) || got["paid_off"] != true {
		t.Fatalf("overpaid debt = %v", got)
	}

	h.expect(h.do(http.MethodPut, base, obj{"name": "Cartao 2", "original_cents": 10000, "start_date": "2026-08-01"}), http.StatusOK)
	h.expect(h.do(http.MethodDelete, base+"/payments/"+id(pay), nil), http.StatusNoContent)
	h.expect(h.do(http.MethodDelete, base, nil), http.StatusNoContent)
}

func TestDebtNeverPaysOffWhenPaymentDoesNotCoverInterest(t *testing.T) {
	h := newHarness(t)
	h.create("/api/debts", obj{"name": "Rotativo", "original_cents": 1000000, "monthly_rate_bp": 200, "monthly_payment_cents": 20000, "start_date": "2026-10-01"})
	debt := decode[[]obj](t, h.do(http.MethodGet, "/api/debts", nil))[0]
	if debt["never_pays_off"] != true {
		t.Fatalf("debt = %v", debt)
	}
	if _, has := debt["payoff_month"]; has {
		t.Fatalf("payoff_month must be absent: %v", debt)
	}
}

func TestDebtWithHugeInterestSinceLongAgoNeverShrinksWithoutPayments(t *testing.T) {
	h := newHarness(t)
	h.create("/api/debts", obj{"name": "Agiota", "original_cents": 100000, "monthly_rate_bp": 10000, "monthly_payment_cents": 0, "start_date": "2023-01-01"})

	debt := decode[[]obj](t, h.do(http.MethodGet, "/api/debts", nil))[0]
	balance := debt["balance_cents"].(float64)
	if balance < 100000 || debt["paid_off"] != false || debt["never_pays_off"] != true {
		t.Fatalf("debt = %v", debt)
	}

	d := decode[dashboardBody](t, h.do(http.MethodGet, "/api/dashboard?from=2026-08&to=2026-10", nil))
	history := d.Debts[0]["history"].([]any)
	previous := 0.0
	for _, raw := range history {
		point := raw.(map[string]any)["balance_cents"].(float64)
		if point < previous {
			t.Fatalf("balance shrank without payment: %v", history)
		}
		previous = point
	}
	if previous != balance || d.KPIs["net_worth_cents"] != -balance {
		t.Fatalf("history end %v, balance %v, kpis %v", previous, balance, d.KPIs)
	}
}

func TestDebtStartingInTheFutureIsNotPaidOff(t *testing.T) {
	h := newHarness(t)
	h.create("/api/debts", obj{"name": "Financiamento", "original_cents": 1000000, "monthly_rate_bp": 0, "monthly_payment_cents": 250000, "start_date": "2027-03-01"})

	debt := decode[[]obj](t, h.do(http.MethodGet, "/api/debts", nil))[0]
	if debt["paid_off"] != false || debt["never_pays_off"] != false || debt["balance_cents"] != float64(1000000) {
		t.Fatalf("future debt = %v", debt)
	}
	if debt["payoff_month"] != "2027-07" || debt["payoff_months"] != float64(9) {
		t.Fatalf("future debt payoff = %v", debt)
	}

	d := decode[dashboardBody](t, h.do(http.MethodGet, "/api/dashboard?from=2026-10&to=2026-10", nil))
	if len(d.Debts[0]["projection"].([]any)) != 4 || d.NetWorth[0]["debts_cents"] != float64(0) {
		t.Fatalf("dashboard debts = %v, net worth = %v", d.Debts, d.NetWorth)
	}
}

func TestDebtStartingInTheFutureDeductsPaymentsAlreadyMade(t *testing.T) {
	h := newHarness(t)
	debt := h.create("/api/debts", obj{"name": "Financiamento", "original_cents": 5000000, "monthly_rate_bp": 100, "monthly_payment_cents": 100000, "start_date": "2027-03-01"})
	h.create("/api/debts/"+id(debt)+"/payments", obj{"date": "2026-10-01", "amount_cents": 1000000})

	got := decode[[]obj](t, h.do(http.MethodGet, "/api/debts", nil))[0]
	if got["balance_cents"] != float64(4000000) || got["paid_off"] != false || got["not_started"] != true {
		t.Fatalf("debt = %v", got)
	}

	d := decode[dashboardBody](t, h.do(http.MethodGet, "/api/dashboard?from=2026-10&to=2026-10", nil))
	series := d.Debts[0]
	first := series["projection"].([]any)[0].(map[string]any)
	if series["balance_cents"] != float64(4000000) || first["month"] != "2027-04" || first["balance_cents"] != float64(3940000) {
		t.Fatalf("series = %v", series)
	}

	h.create("/api/debts/"+id(debt)+"/payments", obj{"date": "2026-10-02", "amount_cents": 4000000})
	got = decode[[]obj](t, h.do(http.MethodGet, "/api/debts", nil))[0]
	if got["balance_cents"] != float64(0) || got["paid_off"] != true {
		t.Fatalf("covered debt = %v", got)
	}
}

func TestStartedDebtIsNotMarkedNotStarted(t *testing.T) {
	h := newHarness(t)
	h.create("/api/debts", obj{"name": "Cartao", "original_cents": 1000, "start_date": "2026-10-01"})
	if got := decode[[]obj](t, h.do(http.MethodGet, "/api/debts", nil))[0]; got["not_started"] != false {
		t.Fatalf("debt = %v", got)
	}
}

func TestDatesWithAbsurdYearsAreRejected(t *testing.T) {
	h := newHarness(t)
	for _, date := range []string{"0026-01-01", "0206-01-01", "2200-01-01"} {
		rec := h.do(http.MethodPost, "/api/debts", obj{"name": "Erro", "original_cents": 1000, "start_date": date})
		h.expectErrorCode(rec, http.StatusBadRequest, "validation_error")
	}
	if got := decode[[]obj](t, h.do(http.MethodGet, "/api/debts", nil)); len(got) != 0 {
		t.Fatalf("nothing must be written: %v", got)
	}
}

func TestStoreFailureLogsOnlyTheErrorKind(t *testing.T) {
	logs := &bytes.Buffer{}
	s := &Server{log: slog.New(slog.NewTextHandler(logs, nil))}
	rec := httptest.NewRecorder()
	s.storeError(rec, httptest.NewRequest(http.MethodGet, "/api/debts", nil), errors.New("constraint failed: name='SEGREDO' amount=4242"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	out := logs.String()
	if strings.Contains(out, "SEGREDO") || strings.Contains(out, "4242") || !strings.Contains(out, "error_kind=store") {
		t.Fatalf("log = %s", out)
	}

	logs.Reset()
	s.storeError(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/debts", nil), context.DeadlineExceeded)
	if !strings.Contains(logs.String(), "error_kind=timeout") {
		t.Fatalf("log = %s", logs.String())
	}
	logs.Reset()
	s.storeError(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/debts", nil), context.Canceled)
	if !strings.Contains(logs.String(), "error_kind=canceled") {
		t.Fatalf("log = %s", logs.String())
	}
}

type dashboardBody struct {
	KPIs     obj   `json:"kpis"`
	NetWorth []obj `json:"net_worth"`
	Cashflow []obj `json:"cashflow"`
	Spending []obj `json:"spending_by_category"`
	Debts    []obj `json:"debts"`
	Invest   []obj `json:"investments"`
	Goals    []obj `json:"goals"`
}

func seedDashboard(h *harness) {
	acc, cat := h.accountAndCategory()
	cats := decode[[]obj](h.t, h.do(http.MethodGet, "/api/categories", nil))
	other := int64(cats[1]["id"].(float64))
	h.create("/api/transactions", obj{"type": "income", "amount_cents": 500000, "date": "2026-09-05", "account_id": acc, "category_id": cat})
	h.create("/api/transactions", obj{"type": "expense", "amount_cents": 20000, "date": "2026-09-10", "account_id": acc, "category_id": cat})
	h.create("/api/transactions", obj{"type": "expense", "amount_cents": 5000, "date": "2026-09-12", "account_id": acc, "category_id": other})
	h.create("/api/transactions", obj{"type": "expense", "amount_cents": 10000, "date": "2026-10-01", "account_id": acc, "category_id": cat})

	inv := h.create("/api/investments", obj{"name": "CDB", "type": "fixed_income"})
	h.create("/api/investments/"+id(inv)+"/contributions", obj{"date": "2026-09-15", "amount_cents": 100000})

	debt := h.create("/api/debts", obj{"name": "Emprestimo", "original_cents": 100000, "monthly_payment_cents": 25000, "start_date": "2026-09-01"})
	h.create("/api/debts/"+id(debt)+"/payments", obj{"date": "2026-09-20", "amount_cents": 25000})
}

func TestDashboardCashflowAndSpendingByCategory(t *testing.T) {
	h := newHarness(t)
	seedDashboard(h)

	rec := h.do(http.MethodGet, "/api/dashboard?from=2026-09&to=2026-10", nil)
	h.expect(rec, http.StatusOK)
	d := decode[dashboardBody](t, rec)

	if len(d.Cashflow) != 2 {
		t.Fatalf("cashflow = %v", d.Cashflow)
	}
	sept := d.Cashflow[0]
	if sept["month"] != "2026-09" || sept["income_cents"] != float64(500000) || sept["expense_cents"] != float64(25000) || sept["balance_cents"] != float64(475000) {
		t.Fatalf("september cashflow = %v", sept)
	}
	if len(d.Spending) != 3 || d.Spending[0]["month"] != "2026-09" || d.Spending[0]["amount_cents"] != float64(20000) || d.Spending[1]["amount_cents"] != float64(5000) || d.Spending[2]["month"] != "2026-10" {
		t.Fatalf("spending = %v", d.Spending)
	}
	for _, key := range []string{"net_worth", "cashflow", "spending_by_category", "debts", "investments", "goals", "kpis"} {
		if !strings.Contains(rec.Body.String(), `"`+key+`"`) {
			t.Errorf("dashboard missing %s", key)
		}
	}
}

func TestDashboardNetWorthIsAccountsPlusInvestmentsMinusDebts(t *testing.T) {
	h := newHarness(t)
	seedDashboard(h)

	d := decode[dashboardBody](t, h.do(http.MethodGet, "/api/dashboard?from=2026-09&to=2026-10", nil))
	sept := d.NetWorth[0]
	if sept["accounts_cents"] != float64(475000) || sept["investments_cents"] != float64(100000) || sept["debts_cents"] != float64(75000) || sept["net_worth_cents"] != float64(500000) {
		t.Fatalf("september = %v", sept)
	}
	if d.NetWorth[1]["net_worth_cents"] != float64(490000) {
		t.Fatalf("october = %v", d.NetWorth[1])
	}
	if d.KPIs["net_worth_cents"] != float64(490000) || d.KPIs["month_expenses_cents"] != float64(10000) || d.KPIs["total_debt_cents"] != float64(75000) {
		t.Fatalf("kpis = %v", d.KPIs)
	}
}

func TestDashboardPeriodFiltering(t *testing.T) {
	h := newHarness(t)
	seedDashboard(h)

	d := decode[dashboardBody](t, h.do(http.MethodGet, "/api/dashboard?from=2026-10&to=2026-10", nil))
	if len(d.NetWorth) != 1 || len(d.Cashflow) != 1 || d.Cashflow[0]["month"] != "2026-10" {
		t.Fatalf("filtered dashboard = %+v", d)
	}
	all := decode[dashboardBody](t, h.do(http.MethodGet, "/api/dashboard", nil))
	if len(all.Cashflow) != 2 {
		t.Fatalf("default range = %v", all.Cashflow)
	}
	nextMonth := h.do(http.MethodGet, "/api/dashboard?from=2026-09&to=2026-11", nil)
	h.expect(nextMonth, http.StatusOK)
	if got := decode[obj](t, nextMonth); got["to"] != "2026-10" || len(got["cashflow"].([]any)) != 2 {
		t.Fatalf("next month must be clamped to the current one: to=%v", got["to"])
	}

	for _, bad := range []string{"?from=2026-13", "?to=hoje", "?from=2026-10&to=2026-09", "?from=1900-01", "?from=2026-10-01", "?from=2026-10&to=2026-12", "?to=2027-05", "?from=2026-11&to=2026-11"} {
		h.expectErrorCode(h.do(http.MethodGet, "/api/dashboard"+bad, nil), http.StatusBadRequest, "validation_error")
	}
}

func TestDashboardEmptyDatabaseIsValid(t *testing.T) {
	h := newHarness(t)
	rec := h.do(http.MethodGet, "/api/dashboard", nil)
	h.expect(rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), "null") {
		t.Fatalf("empty dashboard contains null: %s", rec.Body.String())
	}
}

func TestOversizedBodyIsRejected(t *testing.T) {
	h := newHarness(t)
	big := `{"name":"` + strings.Repeat("a", 2<<20) + `","type":"cash"}`
	h.expectErrorCode(h.do(http.MethodPost, "/api/accounts", big), http.StatusRequestEntityTooLarge, "payload_too_large")
}

func TestWritesRequireJSONContentType(t *testing.T) {
	h := newHarness(t)
	req := httptest.NewRequest(http.MethodPost, "/api/accounts", strings.NewReader(`{"name":"x","type":"cash"}`))
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	h.expectErrorCode(rec, http.StatusUnsupportedMediaType, "unsupported_media_type")
}

func TestForeignHostIsRejected(t *testing.T) {
	h := newHarness(t)
	for _, host := range []string{"evil.example", "evil.example:8080"} {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.handler.ServeHTTP(rec, req)
		h.expectErrorCode(rec, http.StatusForbidden, "forbidden")
	}
	for _, host := range []string{"localhost:8080", "127.0.0.1:9", "[::1]:8080", "localhost"} {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.handler.ServeHTTP(rec, req)
		h.expect(rec, http.StatusOK)
	}
}

func TestUnregisteredAPIMethodUsesTheErrorEnvelope(t *testing.T) {
	h := newHarness(t)
	h.expectErrorCode(h.do(http.MethodPatch, "/api/accounts/1", obj{"name": "x"}), http.StatusMethodNotAllowed, "method_not_allowed")
	h.expectErrorCode(h.do(http.MethodDelete, "/api/dashboard", nil), http.StatusMethodNotAllowed, "method_not_allowed")
	h.expectErrorCode(h.do(http.MethodGet, "/api/nao-existe", nil), http.StatusNotFound, "not_found")
}

func TestPanicBecomesGenericInternalError(t *testing.T) {
	logs := &bytes.Buffer{}
	s := &Server{log: slog.New(slog.NewTextHandler(logs, nil))}
	handler := s.recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("secret internal detail 4242")
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"internal"`) {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "4242") || strings.Contains(logs.String(), "4242") {
		t.Fatalf("panic details leaked: body=%s logs=%s", rec.Body.String(), logs.String())
	}
}

func TestStoreFailureReturnsInternalWithoutDetails(t *testing.T) {
	h := newHarness(t)
	db, _ := store.Open(":memory:")
	_ = db.Close()
	broken := New(Config{Store: db, Now: func() time.Time { return fixedNow }})

	req := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()
	broken.ServeHTTP(rec, req)
	h.expectErrorCode(rec, http.StatusInternalServerError, "internal")
	if strings.Contains(strings.ToLower(rec.Body.String()), "sql") {
		t.Fatalf("internal detail leaked: %s", rec.Body.String())
	}
}

func TestRequestLogContainsOnlyMethodRouteStatusDuration(t *testing.T) {
	h := newHarness(t)
	acc, cat := h.accountAndCategory()
	h.create("/api/transactions", obj{"type": "expense", "amount_cents": 987654, "date": "2026-10-01", "account_id": acc, "category_id": cat, "description": "SEGREDO-FINANCEIRO"})

	logs := h.logs.String()
	for _, want := range []string{"method=POST", "route=/api/transactions", "status=201", "duration"} {
		if !strings.Contains(logs, want) {
			t.Errorf("log missing %q in:\n%s", want, logs)
		}
	}
	for _, leaked := range []string{"SEGREDO", "987654"} {
		if strings.Contains(logs, leaked) {
			t.Errorf("log leaked %q", leaked)
		}
	}
}

func TestInvalidJSONShapes(t *testing.T) {
	h := newHarness(t)
	h.expectErrorCode(h.do(http.MethodPost, "/api/accounts", `{"name":"x","type":"cash"} {"extra":1}`), http.StatusBadRequest, "validation_error")
	h.expectErrorCode(h.do(http.MethodPost, "/api/accounts", `{"name": 5, "type":"cash"}`), http.StatusBadRequest, "validation_error")
}
