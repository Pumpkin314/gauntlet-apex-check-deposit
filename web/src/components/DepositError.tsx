// Error code → investor-visible messages per PRD Section 10
const ERROR_MESSAGES: Record<string, string> = {
  VSS_IQA_BLUR: 'Image too blurry — hold steady on a flat surface with good lighting.',
  VSS_IQA_GLARE: 'Glare detected — avoid direct light on the check.',
  VSS_DUPLICATE_DETECTED: 'This check has already been deposited.',
  FS_OVER_DEPOSIT_LIMIT: 'Maximum single deposit is $5,000. Contact support for larger amounts.',
  FS_ACCOUNT_INELIGIBLE: 'Check deposits are not available for this account type.',
  FS_DUPLICATE_DEPOSIT: 'A similar deposit was recently submitted.',
  SYS_VENDOR_TIMEOUT: "We're experiencing delays. Please try again shortly.",
}

// Only IQA image-quality errors can be retried by retaking the photo
const IQA_RETRYABLE = new Set(['VSS_IQA_BLUR', 'VSS_IQA_GLARE'])

interface DepositErrorProps {
  errorCode: string
  /** Prefer server-supplied user_msg; falls back to local mapping. */
  userMsg?: string
  /** vendor duplicate_original_tx_id for VSS_DUPLICATE_DETECTED */
  duplicateRef?: string
  /** Called when user clicks "Try Again" — only rendered for IQA errors */
  onRetry?: () => void
}

export default function DepositError({ errorCode, userMsg, duplicateRef, onRetry }: DepositErrorProps) {
  const message = userMsg || ERROR_MESSAGES[errorCode] || 'An error occurred. Please try again.'
  const canRetry = IQA_RETRYABLE.has(errorCode)

  return (
    <div
      role="alert"
      style={{
        background: '#fff0f0',
        border: '1px solid #f5c2c7',
        borderRadius: '8px',
        padding: '1rem 1.25rem',
        marginTop: '1rem',
      }}
    >
      <p style={{ margin: 0, fontWeight: 600, color: '#b02a37' }}>Deposit not accepted</p>
      <p style={{ margin: '0.4rem 0 0', color: '#58151c' }}>{message}</p>

      {duplicateRef && (
        <p style={{ margin: '0.4rem 0 0', fontSize: '0.85rem', color: '#58151c' }}>
          Original deposit reference: <code>{duplicateRef}</code>
        </p>
      )}

      {canRetry && onRetry && (
        <button
          onClick={onRetry}
          style={{
            marginTop: '0.75rem',
            padding: '0.45rem 1.1rem',
            background: '#b02a37',
            color: '#fff',
            border: 'none',
            borderRadius: '6px',
            cursor: 'pointer',
            fontWeight: 600,
          }}
        >
          Try Again
        </button>
      )}
    </div>
  )
}
