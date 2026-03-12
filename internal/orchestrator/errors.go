package orchestrator

// DepositError is the typed error structure per PRD Section 10.
// Code is machine-readable (persisted to DB), Message is operator-visible,
// UserMsg is investor-visible and actionable, Detail carries structured metadata.
type DepositError struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	UserMsg string                 `json:"user_message"`
	Detail  map[string]interface{} `json:"detail,omitempty"`
}

func (e *DepositError) Error() string {
	return e.Code + ": " + e.Message
}

// Error code constants — VSS origin.
const (
	ErrCodeVSSIQABlur          = "VSS_IQA_BLUR"
	ErrCodeVSSIQAGlare         = "VSS_IQA_GLARE"
	ErrCodeVSSMICRReadFail     = "VSS_MICR_READ_FAIL"
	ErrCodeVSSDuplicateDetected = "VSS_DUPLICATE_DETECTED"
	ErrCodeVSSAmountMismatch   = "VSS_AMOUNT_MISMATCH"
)

// Error code constants — Funding Service origin.
const (
	ErrCodeFSOverDepositLimit = "FS_OVER_DEPOSIT_LIMIT"
	ErrCodeFSAccountIneligible = "FS_ACCOUNT_INELIGIBLE"
	ErrCodeFSDuplicateDeposit = "FS_DUPLICATE_DEPOSIT"
)

// Error code constants — System origin.
const (
	ErrCodeSysVendorTimeout    = "SYS_VENDOR_TIMEOUT"
	ErrCodeSysLedgerPostFail   = "SYS_LEDGER_POST_FAIL"
	ErrCodeSysInvalidTransition = "SYS_INVALID_TRANSITION"
)

// errorMessages maps error codes to their user-facing and operator messages.
var errorMessages = map[string]struct {
	Message string
	UserMsg string
}{
	ErrCodeVSSIQABlur:           {Message: "VSS IQA failure: image blur detected", UserMsg: "Image too blurry — hold steady on a flat surface with good lighting."},
	ErrCodeVSSIQAGlare:          {Message: "VSS IQA failure: glare detected", UserMsg: "Glare detected — avoid direct light on the check."},
	ErrCodeVSSMICRReadFail:      {Message: "VSS MICR read failure — flagged for operator review", UserMsg: ""},
	ErrCodeVSSDuplicateDetected: {Message: "VSS duplicate check detected", UserMsg: "This check has already been deposited."},
	ErrCodeVSSAmountMismatch:    {Message: "VSS OCR amount mismatch — flagged for operator review", UserMsg: ""},
	ErrCodeFSOverDepositLimit:   {Message: "Funding Service: amount exceeds deposit limit", UserMsg: "Maximum single deposit is $5,000. Contact support for larger amounts."},
	ErrCodeFSAccountIneligible:  {Message: "Funding Service: account type ineligible for check deposits", UserMsg: "Check deposits are not available for this account type."},
	ErrCodeFSDuplicateDeposit:   {Message: "Funding Service: duplicate deposit detected", UserMsg: "A similar deposit was recently submitted."},
	ErrCodeSysVendorTimeout:     {Message: "Vendor service timed out", UserMsg: "We're experiencing delays. Please try again shortly."},
	ErrCodeSysLedgerPostFail:    {Message: "Ledger posting failed", UserMsg: ""},
	ErrCodeSysInvalidTransition: {Message: "Invalid state transition (concurrent race)", UserMsg: ""},
}

// NewDepositError creates a DepositError from an error code, looking up messages from the taxonomy.
func NewDepositError(code string, detail map[string]interface{}) *DepositError {
	msgs, ok := errorMessages[code]
	if !ok {
		msgs.Message = code
		msgs.UserMsg = ""
	}
	return &DepositError{
		Code:    code,
		Message: msgs.Message,
		UserMsg: msgs.UserMsg,
		Detail:  detail,
	}
}

// UserMessageForCode returns the user-facing message for a given error code.
// Returns empty string for codes that are internal-only or operator-review.
func UserMessageForCode(code string) string {
	if msgs, ok := errorMessages[code]; ok {
		return msgs.UserMsg
	}
	return ""
}
