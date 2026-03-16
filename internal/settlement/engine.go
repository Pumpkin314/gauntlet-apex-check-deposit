package settlement

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	icl "github.com/moov-io/imagecashletter"
)

// Transfer is the minimal interface the settlement engine needs from a transfer record.
type Transfer struct {
	ID              string
	AccountID       string
	FromAccountID   string
	CorrespondentID string
	Amount          float64
	State           string
	MICRData        map[string]interface{}
	FrontImageRef   *string
	BackImageRef    *string
	SubmittedAt     time.Time
}

// Batch represents a settlement batch for one correspondent.
type Batch struct {
	ID               uuid.UUID
	CorrespondentID  string
	CutoffDate       time.Time
	Status           string // PENDING, SUBMITTED, ACKNOWLEDGED
	FileRef          string
	RecordCount      int
	TotalAmount      float64
	SubmittedAt      *time.Time
	AcknowledgedAt   *time.Time
	CreatedAt        time.Time
	TransferIDs      []string
}

// SettlementFile is the JSON structure per PRD 6.4.
type SettlementFile struct {
	FileHeader FileHeader `json:"file_header"`
	CashLetter CashLetter `json:"cash_letter"`
}

// FileHeader is the top-level header.
type FileHeader struct {
	Sender    string `json:"sender"`
	CreatedAt string `json:"created_at"`
}

// CashLetter groups checks by correspondent.
type CashLetter struct {
	CorrespondentID string   `json:"correspondent_id"`
	CutoffDate      string   `json:"cutoff_date"`
	Bundles         []Bundle `json:"bundles"`
	TotalAmount     float64  `json:"total_amount"`
	RecordCount     int      `json:"record_count"`
}

// Bundle is a group of checks.
type Bundle struct {
	Checks []Check `json:"checks"`
}

// Check is a single check in the settlement file.
type Check struct {
	TransferID    string    `json:"transfer_id"`
	MICR          MICREntry `json:"micr"`
	Amount        float64   `json:"amount"`
	FrontImageRef string    `json:"front_image_ref"`
	BackImageRef  string    `json:"back_image_ref"`
}

// MICREntry holds the MICR data for a check.
type MICREntry struct {
	Routing     string `json:"routing"`
	Account     string `json:"account"`
	CheckNumber string `json:"check_number"`
}

// TransferQuerier queries FundsPosted transfers.
type TransferQuerier interface {
	ListFundsPostedBefore(ctx context.Context, cutoffUTC time.Time) ([]*Transfer, error)
}

// BatchCreator creates and manages settlement batches.
type BatchCreator interface {
	CreateBatch(ctx context.Context, correspondentID string, cutoffDate time.Time, fileRef string, recordCount int, totalAmount float64) (uuid.UUID, error)
	UpdateBatchStatus(ctx context.Context, batchID uuid.UUID, status string, acknowledgedAt *time.Time) error
	SetSettlementBatch(ctx context.Context, transferID string, batchID uuid.UUID, settledAt time.Time) error
	ListBatches(ctx context.Context) ([]*Batch, error)
	GetBatch(ctx context.Context, id uuid.UUID) (*Batch, error)
}

// StateTransitioner transitions transfer state.
type StateTransitioner interface {
	UpdateState(ctx context.Context, transferID string, from, to string) error
}

// EventRecorder writes transfer events.
type EventRecorder interface {
	WriteEvent(ctx context.Context, transferID, step, actor string, data map[string]interface{}) error
}

// Notifier sends pg_notify.
type Notifier interface {
	Notify(ctx context.Context, transferID string, payload map[string]interface{}) error
}

// Engine orchestrates settlement batch generation.
type Engine struct {
	Transfers TransferQuerier
	Batches   BatchCreator
	Updater   StateTransitioner
	Events    EventRecorder
	Notifier  Notifier
	Log       *slog.Logger
	DataDir   string // defaults to ./data/settlement
}

// TriggerResult is the output of a settlement trigger.
type TriggerResult struct {
	BatchCount   int              `json:"batch_count"`
	TotalChecks  int              `json:"total_checks"`
	TotalAmount  float64          `json:"total_amount"`
	CutoffTime   string           `json:"cutoff_time"`
	Batches      []BatchSummary   `json:"batches"`
}

// BatchSummary is a summary of one batch in the trigger result.
type BatchSummary struct {
	BatchID         string  `json:"batch_id"`
	CorrespondentID string  `json:"correspondent_id"`
	RecordCount     int     `json:"record_count"`
	TotalAmount     float64 `json:"total_amount"`
	FileRef         string  `json:"file_ref"`
}

// Trigger generates settlement batches for all FundsPosted transfers before the cutoff.
func (e *Engine) Trigger(ctx context.Context, now time.Time) (*TriggerResult, error) {
	cutoff := CutoffForDate(now.In(CTLocation))
	// If now is past today's cutoff, still use today's cutoff for batch generation
	// (manual trigger collects everything before the cutoff)

	e.Log.InfoContext(ctx, "settlement trigger", "cutoff_utc", cutoff.UTC())

	transfers, err := e.Transfers.ListFundsPostedBefore(ctx, cutoff)
	if err != nil {
		return nil, fmt.Errorf("list FundsPosted transfers: %w", err)
	}

	if len(transfers) == 0 {
		return &TriggerResult{
			CutoffTime: cutoff.UTC().Format(time.RFC3339),
		}, nil
	}

	// Group by correspondent
	grouped := make(map[string][]*Transfer)
	for _, t := range transfers {
		grouped[t.CorrespondentID] = append(grouped[t.CorrespondentID], t)
	}

	dataDir := e.DataDir
	if dataDir == "" {
		dataDir = "./data/settlement"
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create settlement dir: %w", err)
	}

	result := &TriggerResult{
		CutoffTime: cutoff.UTC().Format(time.RFC3339),
	}

	cutoffDate := now.In(CTLocation)

	settlementFormat := os.Getenv("SETTLEMENT_FORMAT")
	if settlementFormat == "" {
		settlementFormat = "json"
	}

	for corrID, txns := range grouped {
		file := e.buildSettlementFile(corrID, cutoffDate, txns)

		// Write file to disk — format selected by SETTLEMENT_FORMAT env var
		var filePath string
		if settlementFormat == "x9" {
			filename := fmt.Sprintf("settlement_%s_%s.x9", corrID[:8], cutoffDate.Format("2006-01-02"))
			filePath = filepath.Join(dataDir, filename)
			if err := writeX9SettlementFile(filePath, file); err != nil {
				e.Log.ErrorContext(ctx, "write X9 settlement file failed", "correspondent_id", corrID, "error", err)
				return nil, fmt.Errorf("write X9 settlement file: %w", err)
			}
		} else {
			filename := fmt.Sprintf("settlement_%s_%s.json", corrID[:8], cutoffDate.Format("2006-01-02"))
			filePath = filepath.Join(dataDir, filename)
			if err := writeSettlementFile(filePath, file); err != nil {
				e.Log.ErrorContext(ctx, "write settlement file failed", "correspondent_id", corrID, "error", err)
				return nil, fmt.Errorf("write settlement file: %w", err)
			}
		}

		// Create batch record
		var totalAmount float64
		transferIDs := make([]string, 0, len(txns))
		for _, t := range txns {
			totalAmount += t.Amount
			transferIDs = append(transferIDs, t.ID)
		}

		batchID, err := e.Batches.CreateBatch(ctx, corrID, cutoffDate, filePath, len(txns), totalAmount)
		if err != nil {
			return nil, fmt.Errorf("create batch: %w", err)
		}

		// Link transfers to batch
		for _, t := range txns {
			_ = e.Batches.SetSettlementBatch(ctx, t.ID, batchID, time.Time{}) // settled_at set on acknowledgment
		}

		result.Batches = append(result.Batches, BatchSummary{
			BatchID:         batchID.String(),
			CorrespondentID: corrID,
			RecordCount:     len(txns),
			TotalAmount:     totalAmount,
			FileRef:         filePath,
		})
		result.BatchCount++
		result.TotalChecks += len(txns)
		result.TotalAmount += totalAmount
	}

	return result, nil
}

// AcknowledgeBatch processes acknowledgment from the settlement bank.
// Transitions all transfers in the batch from FundsPosted → Completed.
func (e *Engine) AcknowledgeBatch(ctx context.Context, batchID uuid.UUID, acknowledgedAt time.Time) error {
	batch, err := e.Batches.GetBatch(ctx, batchID)
	if err != nil {
		return fmt.Errorf("get batch: %w", err)
	}

	if batch.Status == "ACKNOWLEDGED" {
		return nil // idempotent
	}

	// Update batch status
	if err := e.Batches.UpdateBatchStatus(ctx, batchID, "ACKNOWLEDGED", &acknowledgedAt); err != nil {
		return fmt.Errorf("update batch status: %w", err)
	}

	// Transition each transfer FundsPosted → Completed
	for _, tid := range batch.TransferIDs {
		if err := e.Updater.UpdateState(ctx, tid, "FundsPosted", "Completed"); err != nil {
			e.Log.WarnContext(ctx, "transition FundsPosted→Completed failed (may already be transitioned)",
				"transfer_id", tid, "error", err)
			continue
		}

		// Write events
		_ = e.Events.WriteEvent(ctx, tid, "state_changed", "system", map[string]interface{}{
			"from_state": "FundsPosted",
			"to_state":   "Completed",
		})
		_ = e.Events.WriteEvent(ctx, tid, "settlement_completed", "settlement_engine", map[string]interface{}{
			"batch_id":        batchID.String(),
			"acknowledged_at": acknowledgedAt.Format(time.RFC3339),
		})

		// Set settled_at
		_ = e.Batches.SetSettlementBatch(ctx, tid, batchID, acknowledgedAt)

		// Notify
		if e.Notifier != nil {
			_ = e.Notifier.Notify(ctx, tid, map[string]interface{}{
				"transfer_id": tid,
				"from_state":  "FundsPosted",
				"to_state":    "Completed",
			})
		}
	}

	e.Log.InfoContext(ctx, "batch acknowledged",
		"batch_id", batchID.String(),
		"transfer_count", len(batch.TransferIDs),
	)

	return nil
}

func (e *Engine) buildSettlementFile(correspondentID string, cutoffDate time.Time, transfers []*Transfer) SettlementFile {
	checks := make([]Check, 0, len(transfers))
	var total float64

	for _, t := range transfers {
		micr := MICREntry{}
		if t.MICRData != nil {
			if v, ok := t.MICRData["routing"].(string); ok {
				micr.Routing = v
			}
			if v, ok := t.MICRData["account"].(string); ok {
				micr.Account = v
			}
			if v, ok := t.MICRData["check_number"].(string); ok {
				micr.CheckNumber = v
			}
		}

		frontRef := ""
		backRef := ""
		if t.FrontImageRef != nil {
			frontRef = *t.FrontImageRef
		}
		if t.BackImageRef != nil {
			backRef = *t.BackImageRef
		}

		checks = append(checks, Check{
			TransferID:    t.ID,
			MICR:          micr,
			Amount:        t.Amount,
			FrontImageRef: frontRef,
			BackImageRef:  backRef,
		})
		total += t.Amount
	}

	return SettlementFile{
		FileHeader: FileHeader{
			Sender:    "APEX",
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		},
		CashLetter: CashLetter{
			CorrespondentID: correspondentID,
			CutoffDate:      cutoffDate.Format("2006-01-02"),
			Bundles:         []Bundle{{Checks: checks}},
			TotalAmount:     total,
			RecordCount:     len(checks),
		},
	}
}

func writeSettlementFile(path string, file SettlementFile) error {
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settlement file: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// writeX9SettlementFile writes an X9.37-format settlement file using moov-io/imagecashletter.
func writeX9SettlementFile(path string, file SettlementFile) error {
	now := time.Now()

	fh := icl.NewFileHeader()
	fh.StandardLevel = "35"
	fh.TestFileIndicator = "T"
	fh.ImmediateDestination = "061000146"
	fh.ImmediateOrigin = "026073150"
	fh.FileCreationDate = now
	fh.FileCreationTime = now
	fh.ResendIndicator = "N"
	fh.ImmediateDestinationName = "Fed Reserve"
	fh.ImmediateOriginName = "APEX"
	fh.CompanionDocumentIndicator = ""

	x9File := icl.NewFile()
	x9File.SetHeader(fh)

	clh := icl.NewCashLetterHeader()
	clh.CollectionTypeIndicator = "01"
	clh.DestinationRoutingNumber = "061000146"
	clh.ECEInstitutionRoutingNumber = "026073150"
	clh.CashLetterBusinessDate = now
	clh.CashLetterCreationDate = now
	clh.CashLetterCreationTime = now
	clh.RecordTypeIndicator = "I"
	clh.DocumentationTypeIndicator = "G"
	clh.CashLetterID = file.CashLetter.CorrespondentID[:8]

	cl := icl.NewCashLetter(clh)

	bh := icl.NewBundleHeader()
	bh.CollectionTypeIndicator = "01"
	bh.DestinationRoutingNumber = "061000146"
	bh.ECEInstitutionRoutingNumber = "026073150"
	bh.BundleBusinessDate = now
	bh.BundleCreationDate = now
	bh.BundleID = "1"
	bh.BundleSequenceNumber = "1"

	bundle := icl.NewBundle(bh)

	for i, check := range file.CashLetter.Bundles[0].Checks {
		cd := icl.NewCheckDetail()
		cd.PayorBankRoutingNumber = check.MICR.Routing
		if len(cd.PayorBankRoutingNumber) >= 9 {
			cd.PayorBankCheckDigit = cd.PayorBankRoutingNumber[8:]
			cd.PayorBankRoutingNumber = cd.PayorBankRoutingNumber[:8]
		} else if len(cd.PayorBankRoutingNumber) < 8 {
			cd.PayorBankRoutingNumber = fmt.Sprintf("%-8s", cd.PayorBankRoutingNumber)
		}
		cd.OnUs = check.MICR.Account
		cd.ItemAmount = int(check.Amount * 100)
		cd.EceInstitutionItemSequenceNumber = fmt.Sprintf("%015d", i+1)
		cd.DocumentationTypeIndicator = "G"
		cd.BOFDIndicator = "Y"
		bundle.AddCheckDetail(cd)
	}

	cl.AddBundle(bundle)

	if err := cl.Create(); err != nil {
		return fmt.Errorf("cash letter create: %w", err)
	}
	x9File.AddCashLetter(cl)

	if err := x9File.Create(); err != nil {
		return fmt.Errorf("file create: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	w := icl.NewWriter(f)
	if err := w.Write(x9File); err != nil {
		return fmt.Errorf("write X9: %w", err)
	}

	return nil
}
