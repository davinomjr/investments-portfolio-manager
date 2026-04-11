# Finance Tracker — Project Plan

Personal finance and spending control app. Separate site, same design language and tech stack as the portfolio manager.

---

## Goal

Get full visibility into where money goes each month, then layer on budgets, alerts, and forecasts to bring spending under control.

---

## Phase 1: Foundation — Transaction Import & Categorization

The core data pipeline. Everything else builds on this.

### 1.1 Bank Statement Import

- Support OFX (most Brazilian banks export this: Nubank, Itau, Bradesco, Inter)
- Support CSV as fallback (manual export from bank apps)
- Parse into a unified transaction model:
  - date, description, amount, type (credit/debit), source bank, raw text
- Python worker (same pattern as B3 worker) handles parsing
- Upload via UI or CLI

### 1.2 Transaction Storage

- SQLite database (single file, WAL mode — same as portfolio manager)
- Tables:
  - `transactions` — date, description, amount, type, bank, category_id, raw_text, import_hash (dedup)
  - `categories` — id, name, color, icon, is_system (pre-seeded defaults)
  - `category_rules` — pattern (regex/substring), category_id, priority (auto-matching)
  - `imports` — import log with filename, date, row count, status

### 1.3 Auto-Categorization

- Rules-based first: match transaction description against `category_rules`
- Pre-seed common Brazilian merchants:
  - "IFOOD", "UBER EATS", "RAPPI" → Food Delivery
  - "UBER TRIP", "99" → Transport
  - "NETFLIX", "SPOTIFY", "AMAZON PRIME" → Subscriptions
  - "FARMACIA", "DROGASIL" → Health
  - etc.
- Manual override in UI → optionally create a new rule from the correction
- No ML in phase 1, just pattern matching

### 1.4 Monthly Spending Breakdown

- Dashboard with:
  - Total spent this month vs. last month
  - Pie/donut chart by category
  - Top 10 transactions list
  - Category breakdown table (category, total, % of spend, transaction count)
- Dark theme, same design tokens (ink, sand, clay, pine, gold)

### Deliverable

Upload a bank statement → see categorized transactions → view monthly spending chart.

---

## Phase 2: Budgets & Tracking

### 2.1 Budget Setting

- Set monthly budget per category (e.g., Food: R$ 2,000)
- Set overall monthly spending cap
- Copy budgets from previous month as starting point

### 2.2 Budget vs. Actual View

- Progress bars per category: green → yellow → red as you approach/exceed budget
- "Days left vs. budget remaining" indicator
- Month-over-month trend lines

### 2.3 Recurring Expense Detection

- Auto-detect transactions that repeat monthly (same merchant, similar amount)
- Surface total monthly fixed costs
- Flag subscriptions for review

---

## Phase 3: Insights & Forecasting

### 3.1 Spending Alerts

- "You've hit 80% of [category] budget with X days left"
- Monthly summary report: top categories, biggest expenses, anomalies
- Anomaly detection: flag categories where spending is 2x+ the 3-month average

### 3.2 Cash Flow Forecast

- Project end-of-month balance based on:
  - Current balance
  - Known recurring expenses remaining
  - Average daily discretionary spending
- Simple projection, not a full simulation

### 3.3 Monte Carlo for Spending (stretch)

- Reuse simulation engine from portfolio manager
- "Given your spending variance, what's the probability you stay within budget?"
- Fun and unique — not something other finance apps do

---

## Phase 4: Integration & Polish

### 4.1 Portfolio Manager Integration

- Unified net worth view: investments + bank balance - upcoming expenses
- "Money saved this month" → direct link to investment opportunities
- Shared auth if both apps are deployed

### 4.2 Brazilian-Specific Features

- PIX transaction recognition
- Boleto due date tracking
- Credit card bill vs. individual transactions reconciliation
- IRPF-friendly category exports (medical, education, etc.)

### 4.3 Multi-Bank Support

- Dashboard showing balances across banks
- Consolidated view with per-bank filtering

---

## Tech Stack

| Layer | Technology | Notes |
|-------|-----------|-------|
| Backend | Go + SQLite (pure Go driver) | Same architecture as portfolio manager |
| Frontend | Next.js + React + Tailwind | Same design system, dark theme |
| Worker | Python | OFX/CSV parsing, same subprocess pattern |
| Database | SQLite (WAL) | Single file, local-first |
| Hosting | Railway (or local-only initially) | Same deployment pattern |

---

## Data Model (Phase 1)

```sql
CREATE TABLE categories (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    color TEXT NOT NULL,        -- hex color for charts
    icon TEXT,                  -- optional icon identifier
    is_system BOOLEAN DEFAULT 0 -- pre-seeded vs user-created
);

CREATE TABLE category_rules (
    id INTEGER PRIMARY KEY,
    pattern TEXT NOT NULL,       -- substring or regex
    category_id INTEGER NOT NULL REFERENCES categories(id),
    priority INTEGER DEFAULT 0,  -- higher wins on conflict
    is_regex BOOLEAN DEFAULT 0
);

CREATE TABLE transactions (
    id INTEGER PRIMARY KEY,
    date TEXT NOT NULL,           -- ISO 8601
    description TEXT NOT NULL,
    amount REAL NOT NULL,         -- negative = expense, positive = income
    type TEXT NOT NULL,           -- 'debit' or 'credit'
    bank TEXT,
    category_id INTEGER REFERENCES categories(id),
    raw_text TEXT,                -- original unparsed line
    import_hash TEXT UNIQUE,     -- SHA256 for dedup
    import_id INTEGER REFERENCES imports(id),
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE imports (
    id INTEGER PRIMARY KEY,
    filename TEXT NOT NULL,
    bank TEXT,
    format TEXT NOT NULL,        -- 'ofx', 'csv'
    row_count INTEGER,
    status TEXT DEFAULT 'pending', -- pending/done/error
    created_at TEXT DEFAULT (datetime('now'))
);
```

---

## Default Categories (Brazilian-focused)

| Category | Example Merchants |
|----------|------------------|
| Food & Groceries | Supermercado, Pao de Acucar, Carrefour |
| Food Delivery | iFood, Uber Eats, Rappi |
| Restaurants | Generic restaurant names |
| Transport | Uber, 99, Shell, Ipiranga |
| Subscriptions | Netflix, Spotify, iCloud, Amazon Prime |
| Health | Farmacia, Drogasil, Unimed |
| Housing | Aluguel, Condominio, CPFL, Sabesp |
| Education | Udemy, Coursera, Alura |
| Shopping | Amazon, Mercado Livre, Shein |
| Travel | Airlines, Booking, Airbnb |
| Transfers | PIX, TED, DOC (neutral, not spending) |
| Income | Salary, freelance, dividends |
| Uncategorized | Default bucket |

---

## Open Questions

- [ ] Separate repo or monorepo with portfolio manager?
- [ ] Shared auth system across both apps?
- [ ] Start local-only or deploy to Railway from day one?
- [ ] Support credit card bills (fatura) as a separate import format?
- [ ] Nubank CSV has a specific format — prioritize it as first bank?
