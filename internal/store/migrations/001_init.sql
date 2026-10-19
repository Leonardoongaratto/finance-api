CREATE TABLE accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('checking', 'savings', 'cash', 'credit_card'))
);

CREATE TABLE categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE transactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL CHECK (type IN ('income', 'expense')),
    amount_cents INTEGER NOT NULL CHECK (amount_cents > 0),
    date TEXT NOT NULL,
    account_id INTEGER NOT NULL REFERENCES accounts (id),
    category_id INTEGER NOT NULL REFERENCES categories (id),
    description TEXT NOT NULL DEFAULT '',
    installment_group TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_transactions_date ON transactions (date);
CREATE INDEX idx_transactions_group ON transactions (installment_group);

CREATE TABLE investments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('fixed_income', 'stocks', 'reits', 'treasury', 'crypto', 'other'))
);

CREATE TABLE investment_contributions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    parent_id INTEGER NOT NULL REFERENCES investments (id) ON DELETE CASCADE,
    date TEXT NOT NULL,
    value_cents INTEGER NOT NULL CHECK (value_cents > 0)
);

CREATE TABLE investment_valuations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    parent_id INTEGER NOT NULL REFERENCES investments (id) ON DELETE CASCADE,
    date TEXT NOT NULL,
    value_cents INTEGER NOT NULL CHECK (value_cents >= 0)
);

CREATE TABLE goals (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    target_cents INTEGER NOT NULL CHECK (target_cents > 0),
    deadline TEXT NOT NULL,
    start_date TEXT NOT NULL
);

CREATE TABLE goal_contributions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    parent_id INTEGER NOT NULL REFERENCES goals (id) ON DELETE CASCADE,
    date TEXT NOT NULL,
    value_cents INTEGER NOT NULL CHECK (value_cents > 0)
);

CREATE TABLE debts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    creditor TEXT NOT NULL DEFAULT '',
    original_cents INTEGER NOT NULL CHECK (original_cents > 0),
    monthly_rate_bp INTEGER NOT NULL DEFAULT 0 CHECK (monthly_rate_bp >= 0),
    monthly_payment_cents INTEGER NOT NULL DEFAULT 0 CHECK (monthly_payment_cents >= 0),
    start_date TEXT NOT NULL
);

CREATE TABLE debt_payments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    parent_id INTEGER NOT NULL REFERENCES debts (id) ON DELETE CASCADE,
    date TEXT NOT NULL,
    value_cents INTEGER NOT NULL CHECK (value_cents > 0)
);

INSERT INTO categories (name) VALUES
    ('Moradia'), ('Alimentação'), ('Transporte'), ('Saúde'), ('Lazer'),
    ('Educação'), ('Assinaturas'), ('Salário'), ('Outros');
