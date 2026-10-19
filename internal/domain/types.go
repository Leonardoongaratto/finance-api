package domain

type AccountType string

const (
	AccountChecking   AccountType = "checking"
	AccountSavings    AccountType = "savings"
	AccountCash       AccountType = "cash"
	AccountCreditCard AccountType = "credit_card"
)

func (t AccountType) Valid() bool {
	switch t {
	case AccountChecking, AccountSavings, AccountCash, AccountCreditCard:
		return true
	}
	return false
}

type TransactionType string

const (
	Income  TransactionType = "income"
	Expense TransactionType = "expense"
)

func (t TransactionType) Valid() bool {
	return t == Income || t == Expense
}

type InvestmentType string

const (
	InvestmentFixedIncome InvestmentType = "fixed_income"
	InvestmentStocks      InvestmentType = "stocks"
	InvestmentREITs       InvestmentType = "reits"
	InvestmentTreasury    InvestmentType = "treasury"
	InvestmentCrypto      InvestmentType = "crypto"
	InvestmentOther       InvestmentType = "other"
)

func (t InvestmentType) Valid() bool {
	switch t {
	case InvestmentFixedIncome, InvestmentStocks, InvestmentREITs, InvestmentTreasury, InvestmentCrypto, InvestmentOther:
		return true
	}
	return false
}

type Account struct {
	ID   int64       `json:"id"`
	Name string      `json:"name"`
	Type AccountType `json:"type"`
}

type Category struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Transaction struct {
	ID               int64           `json:"id"`
	Type             TransactionType `json:"type"`
	AmountCents      int64           `json:"amount_cents"`
	Date             Date            `json:"date"`
	AccountID        int64           `json:"account_id"`
	CategoryID       int64           `json:"category_id"`
	Description      string          `json:"description"`
	InstallmentGroup string          `json:"installment_group,omitempty"`
	SourceRef        string          `json:"source_ref,omitempty"`
}

// SignedCents is the effect of the transaction on its account balance.
func (t Transaction) SignedCents() int64 {
	if t.Type == Expense {
		return -t.AmountCents
	}
	return t.AmountCents
}

type Investment struct {
	ID   int64          `json:"id"`
	Name string         `json:"name"`
	Type InvestmentType `json:"type"`
}

type Contribution struct {
	ID          int64 `json:"id"`
	ParentID    int64 `json:"-"`
	Date        Date  `json:"date"`
	AmountCents int64 `json:"amount_cents"`
}

type Valuation struct {
	ID         int64 `json:"id"`
	ParentID   int64 `json:"-"`
	Date       Date  `json:"date"`
	ValueCents int64 `json:"value_cents"`
}

type Goal struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	TargetCents int64  `json:"target_cents"`
	Deadline    Date   `json:"deadline"`
	StartDate   Date   `json:"start_date"`
}

type Debt struct {
	ID                  int64  `json:"id"`
	Name                string `json:"name"`
	Creditor            string `json:"creditor"`
	OriginalCents       int64  `json:"original_cents"`
	MonthlyRateBP       int64  `json:"monthly_rate_bp"`
	MonthlyPaymentCents int64  `json:"monthly_payment_cents"`
	StartDate           Date   `json:"start_date"`
}

type Payment struct {
	ID          int64 `json:"id"`
	ParentID    int64 `json:"-"`
	Date        Date  `json:"date"`
	AmountCents int64 `json:"amount_cents"`
}
