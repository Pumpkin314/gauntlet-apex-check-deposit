package vendorclient

import "context"

// MICRData holds bank routing information extracted from the check image.
type MICRData struct {
	Routing     string `json:"routing"`
	Account     string `json:"account"`
	CheckNumber string `json:"check_number"`
}

// ValidateRequest is the payload sent to the Vendor Service for image validation.
type ValidateRequest struct {
	AccountID  string  `json:"account_id"`
	Amount     float64 `json:"amount"`
	FrontImage string  `json:"front_image"`
	BackImage  string  `json:"back_image"`
}

// ValidateResponse is the result returned by the Vendor Service after validation.
type ValidateResponse struct {
	IQAStatus             string    `json:"iqa_status"`
	IQAErrorType          *string   `json:"iqa_error_type"`
	MICRData              *MICRData `json:"micr_data"`
	OCRAmount             float64   `json:"ocr_amount"`
	ConfidenceScore       float64   `json:"confidence_score"`
	DuplicateFlag         bool      `json:"duplicate_flag"`
	DuplicateOriginalTxID *string   `json:"duplicate_original_tx_id"`
	TransactionID         string    `json:"transaction_id"`
	ScenarioUsed          string    `json:"scenario_used"`
}

// VendorServiceClient is the interface for calling the Vendor Service.
// Designed as a gRPC extraction seam for Milestone 2.
type VendorServiceClient interface {
	Validate(ctx context.Context, req ValidateRequest) (*ValidateResponse, error)
}
