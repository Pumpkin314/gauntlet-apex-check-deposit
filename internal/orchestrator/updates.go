package orchestrator

import (
	"context"

	"github.com/apex-checkout/check-deposit/internal/vendorclient"
)

// TransferFieldUpdater extends TransferUpdater with methods for updating
// individual fields on the transfer record. Implemented by store.TransferStore.
type TransferFieldUpdater interface {
	TransferUpdater
	SetErrorCode(ctx context.Context, transferID, errorCode string) error
	SetReviewReason(ctx context.Context, transferID, reason string) error
	SetContributionType(ctx context.Context, transferID, ct string) error
	SetVSSResults(ctx context.Context, transferID string, vendorTxID string, confidence float64, micrData map[string]interface{}) error
}

func setErrorCode(ctx context.Context, updater TransferUpdater, transferID, errorCode string) error {
	if u, ok := updater.(TransferFieldUpdater); ok {
		return u.SetErrorCode(ctx, transferID, errorCode)
	}
	return nil
}

func setReviewReason(ctx context.Context, updater TransferUpdater, transferID, reason string) error {
	if u, ok := updater.(TransferFieldUpdater); ok {
		return u.SetReviewReason(ctx, transferID, reason)
	}
	return nil
}

func setContributionType(ctx context.Context, updater TransferUpdater, transferID, ct string) error {
	if u, ok := updater.(TransferFieldUpdater); ok {
		return u.SetContributionType(ctx, transferID, ct)
	}
	return nil
}

func updateTransferVSSResults(ctx context.Context, updater TransferUpdater, transferID string, resp *vendorclient.ValidateResponse) error {
	u, ok := updater.(TransferFieldUpdater)
	if !ok {
		return nil
	}
	var micrData map[string]interface{}
	if resp.MICRData != nil {
		micrData = map[string]interface{}{
			"routing":      resp.MICRData.Routing,
			"account":      resp.MICRData.Account,
			"check_number": resp.MICRData.CheckNumber,
		}
	}
	return u.SetVSSResults(ctx, transferID, resp.TransactionID, resp.ConfidenceScore, micrData)
}
