-- expense_monitor SQLite schema. Run at every startup (idempotent).

CREATE TABLE IF NOT EXISTS accounts (
    account_uid TEXT PRIMARY KEY,
    iban        TEXT,
    name        TEXT,
    currency    TEXT,
    product     TEXT,
    raw_json    TEXT,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS transactions (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    account_uid            TEXT NOT NULL REFERENCES accounts(account_uid),
    dedup_key              TEXT NOT NULL,
    transaction_id         TEXT,
    amount                 REAL NOT NULL,          -- signed: negative for debits (DBIT)
    currency               TEXT,
    credit_debit_indicator TEXT,                   -- CRDT / DBIT
    status                 TEXT,                   -- BOOK / PDNG ...
    booking_date           TEXT,                   -- YYYY-MM-DD
    value_date             TEXT,
    transaction_date       TEXT,
    reference              TEXT,
    remittance_information TEXT,                    -- lines joined by "\n"
    creditor_name          TEXT,
    debtor_name            TEXT,
    raw_json               TEXT,                    -- original transaction JSON
    created_at             TEXT NOT NULL,
    UNIQUE(account_uid, dedup_key)
);

CREATE INDEX IF NOT EXISTS idx_tx_account_booking
    ON transactions(account_uid, booking_date);

CREATE TABLE IF NOT EXISTS sync_state (
    account_uid       TEXT PRIMARY KEY,
    last_synced_at    TEXT,
    last_booking_date TEXT
);

CREATE TABLE IF NOT EXISTS balances (
    account_uid    TEXT NOT NULL REFERENCES accounts(account_uid),
    balance_type   TEXT NOT NULL,           -- CLBD / XPCD / ...
    amount         REAL NOT NULL,
    currency       TEXT,
    reference_date TEXT,
    updated_at     TEXT NOT NULL,
    PRIMARY KEY(account_uid, balance_type)
);

-- Generic key/value store for daemon state (e.g. Telegram notification cursors).
CREATE TABLE IF NOT EXISTS kv (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Manual per-transaction category overrides set from the dashboard.
CREATE TABLE IF NOT EXISTS category_overrides (
    tx_id         INTEGER PRIMARY KEY,     -- transactions.id
    category_name TEXT NOT NULL
);

-- Learned category rules created from the dashboard ("apply to all similar").
-- keyword is a case-insensitive substring, matched like the YAML match_any rules.
CREATE TABLE IF NOT EXISTS learned_rules (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    keyword       TEXT NOT NULL,
    category_name TEXT NOT NULL,
    created_at    TEXT NOT NULL
);
