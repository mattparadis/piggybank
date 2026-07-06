# expense_monitor

Monitoraggio personale delle spese tramite le API di [Enable Banking](https://enablebanking.com/docs/).
Scarica conti e transazioni dal tuo conto bancario e li salva in **SQLite**.

> Questa iterazione implementa **autenticazione + ingestione dati in SQLite** con un
> daemon che sincronizza periodicamente. La **dashboard web** e le **notifiche Telegram**
> sono predisposte (config e punti di aggancio) ma non ancora implementate.

## Come funziona

- **Autenticazione API**: JWT RS256 firmato con la chiave privata `.pem` dell'applicazione
  (header `kid` = application_id).
- **Consenso**: il comando `auth` avvia il flusso, espone un callback **HTTPS** (certificati
  Tailscale), riceve il `code`, crea la sessione e la salva in `session.json`.
- **Sincronizzazione**: il comando `serve` legge la sessione, scarica le transazioni
  (paginazione via `continuation_key`, fetch incrementale) e le salva in SQLite in modo
  idempotente. Se la sessione è scaduta, avvisa di rilanciare `auth`.

## Prerequisiti

1. Account e applicazione su [Enable Banking Control Panel](https://enablebanking.com/docs/api/quick-start/),
   con la chiave privata `.pem` scaricata (nome file = application_id).
2. Nel Control Panel, aggiungi il **redirect URL** alla whitelist, es.
   `https://HOST.TAILNET.ts.net:7777/callback`.
3. Certificato TLS per l'host (Tailscale):
   ```sh
   tailscale cert HOST.TAILNET.ts.net
   # produce HOST.TAILNET.ts.net.crt e HOST.TAILNET.ts.net.key
   ```

## Configurazione

```sh
cp config.example.yaml config.yaml
# poi compila application_id, private_key_path, aspsp_name, redirect_url, cert/key
```

Per trovare il nome esatto della banca (`aspsp_name`) puoi consultare `GET /aspsps?country=IT`.

## Uso (locale)

```sh
go build -o expense_monitor .

# 0. trova il nome esatto della tua banca e mettilo in aspsp_name
./expense_monitor aspsps --config config.yaml

# 1. autorizzazione una tantum (apre il browser, salva session.json)
./expense_monitor auth --config config.yaml

# 2a. sincronizzazione singola (utile per verificare)
./expense_monitor sync-once --config config.yaml

# 2b. daemon con polling periodico (24h / times_per_day)
./expense_monitor serve --config config.yaml
```

## Uso (Docker)

Struttura consigliata delle cartelle (montate come volumi):

```
config.yaml
keys/app.pem
certs/HOST.TAILNET.ts.net.crt
certs/HOST.TAILNET.ts.net.key
data/            # session.json + expense_monitor.db (creati a runtime)
```

I path nel `config.yaml` puntano ai mount: `/keys/...`, `/certs/...`, `/data/...`.

```sh
docker compose build

# 1. autorizzazione una tantum (espone la 7777 per il callback)
docker compose run --rm --service-ports monitor auth --config /config.yaml

# 2. avvio del daemon
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
  budgets:
    - { category: "Groceries", monthly_limit: 400.0 }
```

Open `https://HOST.TAILNET.ts.net:7777/` from a device on your tailnet and log in with
the basic-auth credentials. The dashboard shows an overview (balances, monthly cash flow,
spending by category with budget bars, recent transactions) and a searchable/filterable
transaction list. Categories and budgets are recomputed from the YAML on every request —
no reindexing needed when you change the rules.

To run only the dashboard (no sync): `expense_monitor dashboard --config config.yaml`.

> **Operational note:** the dashboard and the `auth` callback share port 7777. To
> re-authorize when the session expires, stop the daemon first
> (`docker compose stop`), run `auth`, then start it again.

## Ispezionare i dati

```sh
sqlite3 data/expense_monitor.db \
  "SELECT booking_date, amount, currency, credit_debit_indicator, reference
     FROM transactions ORDER BY booking_date DESC LIMIT 10;"

sqlite3 data/expense_monitor.db \
  "SELECT count(*), min(booking_date), max(booking_date) FROM transactions;"
```

## Struttura del progetto

```
main.go                     dispatch sottocomandi (auth | serve | sync-once | dashboard | aspsps)
internal/config             caricamento/validazione config YAML
internal/session            persistenza session.json
internal/enablebanking      client API (JWT RS256, endpoint, saldi)
internal/store              SQLite (schema, upsert idempotente, query di lettura, sync_state)
internal/auth               comando auth (callback HTTPS + creazione sessione)
internal/syncer             motore di sincronizzazione incrementale (transazioni + saldi)
internal/category           categorizzazione delle transazioni da regole YAML
internal/dashboard          dashboard web (html/template + htmx + Chart.js embeddati)
internal/server             daemon serve (scheduler + dashboard; telegram: TODO)
internal/notify             interfaccia notifiche (log ora; Telegram: TODO)
```

## Test

```sh
go test ./...
```
