# expense_monitor

Personal expense monitoring via the [Enable Banking](https://enablebanking.com/docs/) API.
It downloads accounts and transactions from your bank and stores them in **SQLite**, serves
a **web dashboard**, and pushes **Telegram** notifications.

## How it works

- **API authentication**: JWT RS256 signed with the application's `.pem` private key
  (`kid` header = application_id).
- **Consent**: the `auth` command starts the flow, exposes an **HTTPS** callback (Tailscale
  certificates), receives the `code`, creates the session and saves it to `session.json`.
- **Sync**: the `serve` command reads the session, downloads transactions (pagination via
  `continuation_key`, incremental fetch) and stores them in SQLite idempotently. If the
  session has expired, it warns you to run `auth` again.
- **Dashboard & alerts**: `serve` also hosts the web dashboard and evaluates Telegram
  notification rules after each sync.

## Prerequisites

1. Account and application on the [Enable Banking Control Panel](https://enablebanking.com/docs/api/quick-start/),
   with the `.pem` private key downloaded (file name = application_id).
2. In the Control Panel, add the **redirect URL** to the whitelist, e.g.
   `https://HOST.TAILNET.ts.net:7777/callback`.
3. TLS certificate for the host (Tailscale):
   ```sh
   tailscale cert HOST.TAILNET.ts.net
   # produces HOST.TAILNET.ts.net.crt and HOST.TAILNET.ts.net.key
   ```

## Configuration

```sh
cp config.example.yaml config.yaml
# then fill in application_id, private_key_path, aspsp_name, redirect_url, cert/key
```

To find the exact bank name (`aspsp_name`) you can query `GET /aspsps?country=IT`.

## Usage (local)

```sh
go build -o expense_monitor .

# 0. find the exact name of your bank and put it in aspsp_name
./expense_monitor aspsps --config config.yaml

# 1. one-time authorization (opens the browser, saves session.json)
./expense_monitor auth --config config.yaml

# 2a. single sync (handy to verify)
./expense_monitor sync-once --config config.yaml

# 2b. daemon with periodic polling (24h / times_per_day)
./expense_monitor serve --config config.yaml
```

## Usage (Docker)

Recommended folder layout (mounted as volumes):

```
config.yaml
keys/app.pem
certs/HOST.TAILNET.ts.net.crt
certs/HOST.TAILNET.ts.net.key
data/            # session.json + expense_monitor.db (created at runtime)
```

The paths in `config.yaml` point at the mounts: `/keys/...`, `/certs/...`, `/data/...`.

```sh
docker compose build

# 1. one-time authorization (exposes 7777 for the callback)
docker compose run --rm --service-ports monitor auth --config /config.yaml

# 2. start the daemon
docker compose up -d
docker compose logs -f
```

## Web dashboard

The `serve` daemon also hosts a web dashboard on `auth_server.listen_addr` (port
**7777**) over HTTPS (reusing the Tailscale certificate), protected by **basic auth**.
Enable and configure it under `dashboard:` in `config.yaml`:

```yaml
dashboard:
  enabled: true
  basic_auth: { username: "me", password: "change-me" }
  categories:          # first matching keyword wins (case-insensitive)
    - { name: "Groceries", color: "#4caf50", icon: "🛒", match_any: ["ESSELUNGA","COOP"] }
    - { name: "Savings", color: "#795548", icon: "🏦", savings: true, match_any: ["GIROCONTO"] }
  budgets:
    - { category: "Groceries", monthly_limit: 400.0 }
```

Open `https://HOST.TAILNET.ts.net:7777/` from a device on your tailnet and log in with
the basic-auth credentials. The dashboard shows an overview (balances, monthly cash flow,
spending by category with budget bars, recent transactions) and a searchable/filterable
transaction list. Categories and budgets are recomputed from the YAML on every request —
no reindexing needed when you change the rules.

To run only the dashboard (no sync): `expense_monitor dashboard --config config.yaml`.

### Re-categorizing from the dashboard

Categories default to the YAML keyword rules, but you can re-tag transactions from the UI
without editing YAML — the changes are stored as data (SQLite), and the YAML rules stay the
default. Resolution priority per transaction is: **manual override → learned rule → YAML
rule → Uncategorized**.

- On the **Transactions** list and the Overview **Recent** table, each row's category is a
  dropdown. Pick a category to set a **manual override** for just that transaction; pick
  **↺ Auto** to revert to the rules.
- The **≡** button next to it opens "apply to all similar": it stores a **learned rule**
  (keyword → category, case-insensitive substring) that re-tags all matching transactions,
  now and in future syncs. The keyword is pre-filled from the transaction and editable.
- Learned rules are listed under **Custom category rules** on the Overview, each with a
  remove (✕) button.

Overrides and learned rules also apply to the Telegram spending alerts and monthly report.

> **Operational note:** the dashboard and the `auth` callback share port 7777. To
> re-authorize when the session expires, stop the daemon first
> (`docker compose stop`), run `auth`, then start it again.

## Savings category

Mark a category with `savings: true` to treat matching transactions as **money moved to
savings** rather than spending. Savings are excluded from spending totals, budget bars and
the Telegram spending threshold, and are surfaced separately as a "Saved" figure on the
dashboard overview and in the monthly report. This is a read-time rule — retag a category
anytime by editing the YAML, no reindexing needed.

## Telegram notifications

The `serve` daemon can push notifications via the Telegram Bot API. Create a bot with
[@BotFather](https://t.me/BotFather), get your chat id, and configure `telegram:` in
`config.yaml`:

```yaml
telegram:
  enabled: true
  bot_token: "123456:ABC-DEF..."
  chat_id: "12345678"
  startup_ping: true            # one-time "daemon started" confirmation on boot
  session_alerts: true          # session expiry + sync failure alerts
  spending_alert:
    enabled: true
    threshold: 200.0            # first alert when monthly spend (savings excluded) crosses this
    step: 30.0                  # then again every +step: 230, 260, 290, ...
  monthly_report:
    enabled: true
    with_chart: true            # attach a category spending bar chart image
```

Notification types:

- **Startup ping** — one message on boot confirming the bot is configured.
- **Spending alert** — when cumulative monthly spending (savings excluded) crosses
  `threshold`, and again for every additional `step`. The first sync of each month
  establishes a silent baseline, so the initial historical backfill never fires an alert.
- **Session/sync alerts** — when the Enable Banking session expires or a sync fails.
- **Monthly report** — at the start of each new month, a summary of the month that just
  ended (spent / saved / income / top categories), optionally with a bar-chart image.

The `serve` daemon also listens for commands sent in the chat (only from the configured
`chat_id`), shown in the bot's command menu:

- `/report [YYYY-MM]` — send the monthly report now (default: current month).
- `/spending` — current month's spending / saved / income so far.
- `/help` — list the commands.

The same report can be triggered without Telegram via the CLI:
`expense_monitor report --config config.yaml [--month YYYY-MM]`.

## Inspecting the data

```sh
sqlite3 data/expense_monitor.db \
  "SELECT booking_date, amount, currency, credit_debit_indicator, reference
     FROM transactions ORDER BY booking_date DESC LIMIT 10;"

sqlite3 data/expense_monitor.db \
  "SELECT count(*), min(booking_date), max(booking_date) FROM transactions;"
```

## Project structure

```
main.go                     subcommand dispatch (auth | serve | sync-once | dashboard | aspsps)
internal/config             load/validate YAML config
internal/session            session.json persistence
internal/enablebanking      API client (JWT RS256, endpoints, balances)
internal/store              SQLite (schema, idempotent upsert, read queries, sync_state, kv)
internal/auth               auth command (HTTPS callback + session creation)
internal/syncer             incremental sync engine (transactions + balances)
internal/category           transaction categorization from YAML rules (+ savings-aware summary)
internal/dashboard          web dashboard (html/template + htmx + embedded Chart.js)
internal/telegram           Telegram notifications (client, spending alerts, monthly report)
internal/server             serve daemon (scheduler + dashboard + Telegram)
internal/notify             notification interface (log)
```

## Test

```sh
go test ./...
```
