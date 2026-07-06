-- Schema SQLite di expense_monitor. Eseguito ad ogni avvio (idempotente).

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
    amount                 REAL NOT NULL,          -- con segno: negativo per addebiti (DBIT)
    currency               TEXT,
    credit_debit_indicator TEXT,                   -- CRDT / DBIT
    status                 TEXT,                   -- BOOK / PDNG ...
    booking_date           TEXT,                   -- YYYY-MM-DD
    value_date             TEXT,
    transaction_date       TEXT,
    reference              TEXT,
    remittance_information TEXT,                    -- righe unite da "\n"
    creditor_name          TEXT,
    debtor_name            TEXT,
    raw_json               TEXT,                    -- JSON originale della transazione
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
