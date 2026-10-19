package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Leonardoongaratto/finance-api/internal/domain"
	"github.com/Leonardoongaratto/finance-api/internal/report"
	"github.com/Leonardoongaratto/finance-api/internal/store"
)

func (s *Server) listTransactions(w http.ResponseWriter, r *http.Request) {
	filter, ok := parseTransactionFilter(w, r)
	if !ok {
		return
	}
	transactions, err := s.store.ListTransactions(r.Context(), filter)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, transactions)
}

func parseTransactionFilter(w http.ResponseWriter, r *http.Request) (store.TransactionFilter, bool) {
	query := r.URL.Query()
	var filter store.TransactionFilter
	var err error

	if filter.From, err = optionalDate(query.Get("from")); err != nil {
		writeValidation(w, "from must be YYYY-MM-DD")
		return filter, false
	}
	if filter.To, err = optionalDate(query.Get("to")); err != nil {
		writeValidation(w, "to must be YYYY-MM-DD")
		return filter, false
	}
	if filter.AccountID, err = optionalID(query.Get("account_id")); err != nil {
		writeValidation(w, "account_id must be a positive integer")
		return filter, false
	}
	if filter.CategoryID, err = optionalID(query.Get("category_id")); err != nil {
		writeValidation(w, "category_id must be a positive integer")
		return filter, false
	}
	filter.Type = domain.TransactionType(query.Get("type"))
	if filter.Type != "" && !filter.Type.Valid() {
		writeValidation(w, "type must be income or expense")
		return filter, false
	}
	return filter, true
}

func optionalDate(raw string) (domain.Date, error) {
	if raw == "" {
		return "", nil
	}
	return domain.ParseDate(raw)
}

func optionalID(raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err == nil && id <= 0 {
		err = strconv.ErrRange
	}
	return id, err
}

func (s *Server) createInstallments(w http.ResponseWriter, r *http.Request) {
	var plan domain.InstallmentPlan
	if !s.decode(w, r, &plan) {
		return
	}
	if err := validateInstallmentPlan(&plan); err != nil {
		writeValidation(w, err.Error())
		return
	}
	group, err := newGroupID()
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	transactions, err := domain.BuildInstallments(plan, group)
	if err != nil {
		writeValidation(w, err.Error())
		return
	}
	created, err := s.store.CreateTransactions(r.Context(), transactions)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func newGroupID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func (s *Server) deleteInstallmentGroup(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteInstallmentGroup(r.Context(), chi.URLParam(r, "group")); err != nil {
		s.storeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type investmentView struct {
	domain.Investment
	domain.InvestmentSummary
	Contributions []domain.Contribution `json:"contributions"`
	Valuations    []domain.Valuation    `json:"valuations"`
}

func (s *Server) listInvestments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	investments, err := s.store.ListInvestments(ctx)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	contributions, err := s.store.ListInvestmentContributions(ctx)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	valuations, err := s.store.ListInvestmentValuations(ctx)
	if err != nil {
		s.storeError(w, r, err)
		return
	}

	views := make([]investmentView, len(investments))
	for i, inv := range investments {
		view := investmentView{Investment: inv, Contributions: []domain.Contribution{}, Valuations: []domain.Valuation{}}
		for _, c := range contributions {
			if c.ParentID == inv.ID {
				view.Contributions = append(view.Contributions, c)
			}
		}
		for _, v := range valuations {
			if v.ParentID == inv.ID {
				view.Valuations = append(view.Valuations, v)
			}
		}
		view.InvestmentSummary = domain.SummarizeInvestment(view.Contributions, view.Valuations, s.today())
		views[i] = view
	}
	writeJSON(w, http.StatusOK, views)
}

type goalView struct {
	domain.Goal
	domain.GoalProgress
	Contributions []domain.Contribution `json:"contributions"`
}

func (s *Server) listGoals(w http.ResponseWriter, r *http.Request) {
	goals, err := s.store.ListGoals(r.Context())
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	contributions, err := s.store.ListGoalContributions(r.Context())
	if err != nil {
		s.storeError(w, r, err)
		return
	}

	views := make([]goalView, len(goals))
	for i, g := range goals {
		view := goalView{Goal: g, Contributions: []domain.Contribution{}}
		var saved int64
		for _, c := range contributions {
			if c.ParentID == g.ID {
				view.Contributions = append(view.Contributions, c)
				saved += c.AmountCents
			}
		}
		view.GoalProgress = domain.EvaluateGoal(g, saved, s.today())
		views[i] = view
	}
	writeJSON(w, http.StatusOK, views)
}

type debtView struct {
	domain.Debt
	Payments     []domain.Payment `json:"payments"`
	BalanceCents int64            `json:"balance_cents"`
	PaidOff      bool             `json:"paid_off"`
	NeverPaysOff bool             `json:"never_pays_off"`
	PayoffMonth  domain.Month     `json:"payoff_month,omitempty"`
	PayoffMonths int              `json:"payoff_months"`
	NotStarted   bool             `json:"not_started"`
}

func (s *Server) listDebts(w http.ResponseWriter, r *http.Request) {
	debts, err := s.store.ListDebts(r.Context())
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	payments, err := s.store.ListDebtPayments(r.Context())
	if err != nil {
		s.storeError(w, r, err)
		return
	}

	month := s.today().Month()
	views := make([]debtView, len(debts))
	for i, d := range debts {
		view := debtView{Debt: d, Payments: []domain.Payment{}}
		for _, p := range payments {
			if p.ParentID == d.ID {
				view.Payments = append(view.Payments, p)
			}
		}
		balance, projection := domain.DebtOutlook(d, view.Payments, month)
		view.BalanceCents = balance
		view.PaidOff = projection.PaidOff
		view.NeverPaysOff = projection.NeverPaysOff
		view.PayoffMonth = projection.PayoffMonth
		view.PayoffMonths = projection.Months
		view.NotStarted = d.StartsAfter(month)
		views[i] = view
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	query := report.Query{Today: s.today()}
	var err error
	if query.From, err = optionalMonth(r.URL.Query().Get("from")); err != nil {
		writeValidation(w, "from must be YYYY-MM")
		return
	}
	if query.To, err = optionalMonth(r.URL.Query().Get("to")); err != nil {
		writeValidation(w, "to must be YYYY-MM")
		return
	}

	data, err := s.loadReportData(r)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	dashboard, err := report.Build(data, query)
	if err != nil {
		writeValidation(w, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dashboard)
}

func optionalMonth(raw string) (domain.Month, error) {
	if raw == "" {
		return "", nil
	}
	return domain.ParseMonth(raw)
}

func (s *Server) loadReportData(r *http.Request) (report.Data, error) {
	ctx := r.Context()
	var data report.Data
	var err error
	if data.Categories, err = s.store.ListCategories(ctx); err != nil {
		return data, err
	}
	if data.Transactions, err = s.store.ListTransactions(ctx, store.TransactionFilter{}); err != nil {
		return data, err
	}
	if data.Investments, err = s.store.ListInvestments(ctx); err != nil {
		return data, err
	}
	if data.Contributions, err = s.store.ListInvestmentContributions(ctx); err != nil {
		return data, err
	}
	if data.Valuations, err = s.store.ListInvestmentValuations(ctx); err != nil {
		return data, err
	}
	if data.Goals, err = s.store.ListGoals(ctx); err != nil {
		return data, err
	}
	if data.GoalContributions, err = s.store.ListGoalContributions(ctx); err != nil {
		return data, err
	}
	if data.Debts, err = s.store.ListDebts(ctx); err != nil {
		return data, err
	}
	data.Payments, err = s.store.ListDebtPayments(ctx)
	return data, err
}
