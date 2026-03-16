# Risk Dashboard PRD

## Purpose

Provide Apex administrators with a real-time operational risk overview of the check deposit system. The dashboard surfaces key metrics that help identify problematic correspondents, high-exposure investors, and systemic processing issues before they escalate.

## Key Metrics

### 1. Rejection Rate by Correspondent (Rolling 30 Days)

Percentage of deposits rejected per correspondent over the last 30 days. High rejection rates may indicate poor image quality guidance or suspicious activity.

### 2. Float Exposure by Correspondent

Sum of deposit amounts currently in `Approved` or `FundsPosted` states, grouped by correspondent. Represents funds provisionally credited but not yet settled — the system's outstanding risk.

### 3. Return Rate by Correspondent (Rolling 30 Days)

Percentage of completed deposits that were subsequently returned, per correspondent. Tracks post-settlement risk.

### 4. Top Investors by Outstanding Float

Ranked list of investor accounts with the highest total amounts in `Approved` or `FundsPosted` states. Identifies concentration risk.

### 5. Average Processing Time

Mean elapsed time from `Requested` (submitted_at) to `Completed` (updated_at) for deposits that reached the Completed state. Measures operational efficiency.

## API Contract

### `GET /admin/risk-dashboard`

**Auth:** Admin only (`apex-admin` token; role = `admin` or `apex_admin`)

**Response (200):**

```json
{
  "rejection_rate": [
    { "correspondent_id": "c000...", "rate": 12.5, "total": 40, "rejected": 5 }
  ],
  "float_exposure": [
    { "correspondent_id": "c000...", "amount": 15000.00 }
  ],
  "return_rate": [
    { "correspondent_id": "c000...", "rate": 2.5, "completed": 40, "returned": 1 }
  ],
  "top_investors": [
    { "account_id": "a000...", "amount": 5000.00 }
  ],
  "avg_processing_time_secs": 45.2
}
```

**Response (403):** Non-admin token.

## Frontend Layout

Single-page dashboard at `/admin/risk` with:

1. **Header row:** page title + auto-refresh indicator
2. **Metric cards:** Average processing time (single number)
3. **Tables:** Rejection rate, float exposure, return rate (per correspondent)
4. **Table:** Top investors by float (capped at 10 rows)

## SQL Queries

All queries use the existing `transfers` table. No new tables or indexes required.

### Rejection Rate

```sql
SELECT correspondent_id,
       COUNT(*) AS total,
       COUNT(*) FILTER (WHERE state = 'Rejected') AS rejected,
       COUNT(*) FILTER (WHERE state = 'Rejected') * 100.0 / NULLIF(COUNT(*), 0) AS rate
FROM transfers
WHERE submitted_at > NOW() - INTERVAL '30 days'
GROUP BY correspondent_id
```

### Float Exposure

```sql
SELECT correspondent_id,
       SUM(amount) AS amount
FROM transfers
WHERE state IN ('Approved', 'FundsPosted')
GROUP BY correspondent_id
```

### Return Rate

```sql
SELECT correspondent_id,
       COUNT(*) FILTER (WHERE state IN ('Completed', 'Returned')) AS completed,
       COUNT(*) FILTER (WHERE state = 'Returned') AS returned,
       COUNT(*) FILTER (WHERE state = 'Returned') * 100.0 /
         NULLIF(COUNT(*) FILTER (WHERE state IN ('Completed', 'Returned')), 0) AS rate
FROM transfers
WHERE submitted_at > NOW() - INTERVAL '30 days'
GROUP BY correspondent_id
```

### Top Investors by Float

```sql
SELECT account_id, SUM(amount) AS amount
FROM transfers
WHERE state IN ('Approved', 'FundsPosted')
GROUP BY account_id
ORDER BY SUM(amount) DESC
LIMIT $1
```

### Average Processing Time

```sql
SELECT AVG(EXTRACT(EPOCH FROM updated_at - submitted_at)) AS avg_secs
FROM transfers
WHERE state = 'Completed'
```
