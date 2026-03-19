// Centralized configuration for frontend polling intervals, thresholds, and limits.
// All intervals are in milliseconds. Override via VITE_* env vars where noted.

// ── Polling intervals ────────────────────────────────────────────────────────

/** StatusPage: active deposit polling until terminal state */
export const POLL_STATUS_MS = 2_000

/** FlowPage: settlement health bar */
export const POLL_SETTLEMENT_MS = 5_000

/** LedgerPage: balances + reconciliation check */
export const POLL_LEDGER_MS = 5_000

/** NotificationBell: investor notification polling */
export const POLL_NOTIFICATIONS_MS = 10_000

/** AdminLayout: delay before SSE reconnect after error */
export const SSE_RECONNECT_MS = 3_000

// ── UI limits ────────────────────────────────────────────────────────────────

/** AdminLayout: max SSE events kept in the event log */
export const EVENT_LOG_MAX = 100

// ── Risk badge thresholds ────────────────────────────────────────────────────

/** AdminQueue: deposit amount above this shows "Large deposit" badge */
export const LARGE_DEPOSIT_THRESHOLD = 2_000

/** AdminQueue: confidence score below this shows "Low confidence" badge */
export const LOW_CONFIDENCE_THRESHOLD = 0.9
