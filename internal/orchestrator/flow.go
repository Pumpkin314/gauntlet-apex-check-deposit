package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/apex-checkout/check-deposit/internal/funding"
	"github.com/apex-checkout/check-deposit/internal/ledger"
	"github.com/apex-checkout/check-deposit/internal/vendorclient"
)

// Deps bundles the dependencies the orchestrator needs to drive a deposit
// through the happy-path state machine.
type Deps struct {
	Updater     TransferUpdater
	Events      EventWriter
	Notifier    Notifier
	VSS         vendorclient.VendorServiceClient
	Funding     funding.FundingServiceClient
	Ledger      ledger.LedgerService
	Log         *slog.Logger
}

// TransferDetail provides the data ProcessDeposit needs from the caller.
type TransferDetail struct {
	TransferID      string
	AccountID       uuid.UUID
	CorrespondentID uuid.UUID
	Amount          float64
	AccountType     string
	AccountStatus   string
	ContributionType string
	RulesConfig     funding.RulesConfig
	FrontImageRef   string
	BackImageRef    string
}

// ProcessDeposit drives a transfer from Requested through FundsPosted (happy path).
//
//	Requested → Validating (call VSS)
//	Validating → Analyzing (call Funding Service) or Rejected
//	Analyzing → Approved or Rejected
//	Approved → FundsPosted (post ledger)
func ProcessDeposit(ctx context.Context, d Deps, td TransferDetail) (string, error) {
	log := d.Log.With("transfer_id", td.TransferID, "correspondent_id", td.CorrespondentID)

	// 1. Requested → Validating
	if err := Transition(ctx, d.Updater, d.Events, d.Notifier, td.TransferID, Requested, Validating); err != nil {
		return string(Requested), fmt.Errorf("transition Requested→Validating: %w", err)
	}

	// Write vss_called event
	_ = d.Events.WriteEvent(ctx, td.TransferID, "vss_called", "system", map[string]interface{}{
		"account_id": td.AccountID.String(),
		"amount":     td.Amount,
	})

	// Call VSS
	vssResp, err := d.VSS.Validate(ctx, vendorclient.ValidateRequest{
		AccountID:  td.AccountID.String(),
		Amount:     td.Amount,
		FrontImage: td.FrontImageRef,
		BackImage:  td.BackImageRef,
	})
	if err != nil {
		log.ErrorContext(ctx, "VSS call failed", "error", err)
		return string(Validating), fmt.Errorf("VSS validate: %w", err)
	}

	// Write vss_result event
	vssData := map[string]interface{}{
		"iqa_status":       vssResp.IQAStatus,
		"confidence_score": vssResp.ConfidenceScore,
		"duplicate_flag":   vssResp.DuplicateFlag,
		"transaction_id":   vssResp.TransactionID,
	}
	if vssResp.IQAErrorType != nil {
		vssData["iqa_error_type"] = *vssResp.IQAErrorType
	}
	if vssResp.MICRData != nil {
		micrJSON, _ := json.Marshal(vssResp.MICRData)
		vssData["micr_data"] = string(micrJSON)
	}
	_ = d.Events.WriteEvent(ctx, td.TransferID, "vss_result", "vss", vssData)

	// Update transfer with VSS results
	if err := updateTransferVSSResults(ctx, d.Updater, td.TransferID, vssResp); err != nil {
		log.WarnContext(ctx, "failed to update VSS results on transfer", "error", err)
	}

	// Check VSS outcome
	if vssResp.IQAStatus == "fail" {
		errorCode := ErrCodeVSSIQABlur
		if vssResp.IQAErrorType != nil {
			switch *vssResp.IQAErrorType {
			case "glare", ErrCodeVSSIQAGlare:
				errorCode = ErrCodeVSSIQAGlare
			case "blur", ErrCodeVSSIQABlur:
				errorCode = ErrCodeVSSIQABlur
			}
		}
		if err := setErrorAndReject(ctx, d, td.TransferID, Validating, errorCode); err != nil {
			return string(Validating), err
		}
		return string(Rejected), nil
	}
	if vssResp.DuplicateFlag {
		if err := setErrorAndReject(ctx, d, td.TransferID, Validating, ErrCodeVSSDuplicateDetected); err != nil {
			return string(Validating), err
		}
		return string(Rejected), nil
	}

	// 2. Validating → Analyzing
	if err := Transition(ctx, d.Updater, d.Events, d.Notifier, td.TransferID, Validating, Analyzing); err != nil {
		return string(Validating), fmt.Errorf("transition Validating→Analyzing: %w", err)
	}

	// Determine if flagged by VSS (MICR failure or amount mismatch)
	var reviewReason string
	if vssResp.MICRData == nil || (vssResp.IQAStatus == "pass" && vssResp.MICRData != nil && vssResp.MICRData.Routing == "") {
		reviewReason = "VSS_MICR_READ_FAIL"
	}
	if vssResp.OCRAmount != td.Amount {
		reviewReason = "VSS_AMOUNT_MISMATCH"
	}

	// Call Funding Service
	_ = d.Events.WriteEvent(ctx, td.TransferID, "fs_evaluated", "funding_service", map[string]interface{}{
		"account_id": td.AccountID.String(),
		"amount":     td.Amount,
	})

	fsReq := &funding.EvaluateRequest{
		TransferID:       uuid.MustParse(td.TransferID),
		AccountID:        td.AccountID,
		CorrespondentID:  td.CorrespondentID,
		Amount:           td.Amount,
		AccountType:      td.AccountType,
		AccountStatus:    td.AccountStatus,
		ContributionType: td.ContributionType,
		RulesConfig:      td.RulesConfig,
	}

	fsDecision, err := d.Funding.Evaluate(ctx, fsReq)
	if err != nil {
		log.ErrorContext(ctx, "Funding Service call failed", "error", err)
		return string(Analyzing), fmt.Errorf("Funding Service evaluate: %w", err)
	}

	_ = d.Events.WriteEvent(ctx, td.TransferID, "fs_evaluated", "funding_service", map[string]interface{}{
		"decision":    string(fsDecision.Decision),
		"reason_code": fsDecision.ReasonCode,
	})

	// Handle FS rejection
	if fsDecision.Decision == funding.DecisionReject {
		if err := setErrorAndReject(ctx, d, td.TransferID, Analyzing, fsDecision.ReasonCode); err != nil {
			return string(Analyzing), err
		}
		return string(Rejected), nil
	}

	// Handle FLAG_FOR_REVIEW
	if fsDecision.Decision == funding.DecisionFlagForReview || reviewReason != "" {
		reason := reviewReason
		if fsDecision.ReasonCode != "" {
			reason = fsDecision.ReasonCode
		}
		_ = d.Events.WriteEvent(ctx, td.TransferID, "flagged", "system", map[string]interface{}{
			"review_reason": reason,
		})
		// Stay in Analyzing with review_reason set — appears in operator queue
		if err := setReviewReason(ctx, d.Updater, td.TransferID, reason); err != nil {
			log.WarnContext(ctx, "failed to set review_reason", "error", err)
		}
		return string(Analyzing), nil
	}

	// 3. Analyzing → Approved
	if err := Transition(ctx, d.Updater, d.Events, d.Notifier, td.TransferID, Analyzing, Approved); err != nil {
		return string(Analyzing), fmt.Errorf("transition Analyzing→Approved: %w", err)
	}

	// Update contribution type if set
	if fsDecision.ContributionType != "" {
		_ = setContributionType(ctx, d.Updater, td.TransferID, fsDecision.ContributionType)
	}

	// 4. Approved → FundsPosted (post ledger)
	omnibusID := fsDecision.ResolvedOmnibusID
	transferUUID := uuid.MustParse(td.TransferID)
	amountStr := ledger.Amount(fmt.Sprintf("%.2f", td.Amount))

	if err := d.Ledger.PostDoubleEntry(ctx, omnibusID, td.AccountID, amountStr, "FREE", transferUUID); err != nil {
		log.ErrorContext(ctx, "ledger posting failed", "error", err)
		return string(Approved), fmt.Errorf("ledger post: %w", err)
	}

	_ = d.Events.WriteEvent(ctx, td.TransferID, "ledger_posted", "system", map[string]interface{}{
		"debit_account":  omnibusID.String(),
		"credit_account": td.AccountID.String(),
		"amount":         td.Amount,
	})

	if err := Transition(ctx, d.Updater, d.Events, d.Notifier, td.TransferID, Approved, FundsPosted); err != nil {
		return string(Approved), fmt.Errorf("transition Approved→FundsPosted: %w", err)
	}

	log.InfoContext(ctx, "deposit processed successfully", "final_state", "FundsPosted")
	return string(FundsPosted), nil
}

func setErrorAndReject(ctx context.Context, d Deps, transferID string, from TransferState, errorCode string) error {
	_ = setErrorCode(ctx, d.Updater, transferID, errorCode)
	return Transition(ctx, d.Updater, d.Events, d.Notifier, transferID, from, Rejected)
}
