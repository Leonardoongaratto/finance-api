ALTER TABLE transactions ADD COLUMN source_ref TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX idx_transactions_source_ref
    ON transactions (account_id, source_ref)
    WHERE source_ref <> '';
