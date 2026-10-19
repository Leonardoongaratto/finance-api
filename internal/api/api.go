// Package api exposes the finance data as a JSON HTTP API.
package api

import (
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Leonardoongaratto/finance-api/internal/domain"
	"github.com/Leonardoongaratto/finance-api/internal/store"
)

type Config struct {
	Store       *store.Store
	Now         func() time.Time
	Logger      *slog.Logger
	CORSOrigins []string
}

type Server struct {
	store       *store.Store
	now         func() time.Time
	log         *slog.Logger
	corsOrigins map[string]struct{}
}

func New(cfg Config) http.Handler {
	s := &Server{store: cfg.Store, now: cfg.Now, log: cfg.Logger, corsOrigins: originSet(cfg.CORSOrigins)}
	if s.now == nil {
		s.now = time.Now
	}
	if s.log == nil {
		s.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	r := chi.NewRouter()
	r.Use(s.requestLog, s.recoverer, s.hostGuard, s.cors)
	r.NotFound(notFound)
	r.MethodNotAllowed(methodNotAllowed)
	r.Get("/health", s.health)
	r.Route("/api", s.apiRoutes)
	return r
}

func (s *Server) apiRoutes(r chi.Router) {
	st := s.store

	r.Get("/accounts", listHandler(s, st.ListAccounts))
	r.Post("/accounts", createHandler(s, validateAccount, st.CreateAccount))
	r.Put("/accounts/{id}", updateHandler(s, func(a *domain.Account, id int64) { a.ID = id }, validateAccount, st.UpdateAccount))
	r.Delete("/accounts/{id}", deleteHandler(s, st.DeleteAccount))

	r.Get("/categories", listHandler(s, st.ListCategories))
	r.Post("/categories", createHandler(s, validateCategory, st.CreateCategory))
	r.Put("/categories/{id}", updateHandler(s, func(c *domain.Category, id int64) { c.ID = id }, validateCategory, st.UpdateCategory))
	r.Delete("/categories/{id}", deleteHandler(s, st.DeleteCategory))

	r.Get("/transactions", s.listTransactions)
	r.Post("/transactions", createHandler(s, validateNewTransaction, st.CreateTransaction))
	r.Post("/transactions/installments", s.createInstallments)
	r.Put("/transactions/{id}", updateHandler(s, func(t *domain.Transaction, id int64) { t.ID = id }, validateTransaction, st.UpdateTransaction))
	r.Delete("/transactions/{id}", deleteHandler(s, st.DeleteTransaction))
	r.Delete("/installments/{group}", s.deleteInstallmentGroup)

	r.Get("/investments", s.listInvestments)
	r.Post("/investments", createHandler(s, validateInvestment, st.CreateInvestment))
	r.Put("/investments/{id}", updateHandler(s, func(i *domain.Investment, id int64) { i.ID = id }, validateInvestment, st.UpdateInvestment))
	r.Delete("/investments/{id}", deleteHandler(s, st.DeleteInvestment))
	r.Post("/investments/{id}/contributions", childCreateHandler(s, func(c *domain.Contribution, id int64) { c.ParentID = id }, validateContribution, st.AddInvestmentContribution))
	r.Delete("/investments/{id}/contributions/{childID}", childDeleteHandler(s, st.DeleteInvestmentContribution))
	r.Post("/investments/{id}/valuations", childCreateHandler(s, func(v *domain.Valuation, id int64) { v.ParentID = id }, validateValuation, st.AddInvestmentValuation))
	r.Delete("/investments/{id}/valuations/{childID}", childDeleteHandler(s, st.DeleteInvestmentValuation))

	r.Get("/goals", s.listGoals)
	r.Post("/goals", createHandler(s, s.prepareNewGoal, st.CreateGoal))
	r.Put("/goals/{id}", updateHandler(s, func(g *domain.Goal, id int64) { g.ID = id }, validateGoal, st.UpdateGoal))
	r.Delete("/goals/{id}", deleteHandler(s, st.DeleteGoal))
	r.Post("/goals/{id}/contributions", childCreateHandler(s, func(c *domain.Contribution, id int64) { c.ParentID = id }, validateContribution, st.AddGoalContribution))
	r.Delete("/goals/{id}/contributions/{childID}", childDeleteHandler(s, st.DeleteGoalContribution))

	r.Get("/debts", s.listDebts)
	r.Post("/debts", createHandler(s, validateDebt, st.CreateDebt))
	r.Put("/debts/{id}", updateHandler(s, func(d *domain.Debt, id int64) { d.ID = id }, validateDebt, st.UpdateDebt))
	r.Delete("/debts/{id}", deleteHandler(s, st.DeleteDebt))
	r.Post("/debts/{id}/payments", childCreateHandler(s, func(p *domain.Payment, id int64) { p.ParentID = id }, validatePayment, st.AddDebtPayment))
	r.Delete("/debts/{id}/payments/{childID}", childDeleteHandler(s, st.DeleteDebtPayment))

	r.Get("/dashboard", s.dashboard)

	r.Post("/import/ofx/preview", s.previewOFXImport)
	r.Post("/import/ofx/confirm", s.confirmOFXImport)

	r.NotFound(notFound)
	r.MethodNotAllowed(methodNotAllowed)
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "route not found")
}

func methodNotAllowed(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) today() domain.Date {
	return domain.DateOf(s.now())
}
