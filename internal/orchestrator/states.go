package orchestrator

import "fmt"

// TransferState is the type-safe state of a check deposit transfer.
type TransferState string

// The 8 states from the spec. Do not add, remove, or rename.
const (
	Requested   TransferState = "Requested"
	Validating  TransferState = "Validating"
	Analyzing   TransferState = "Analyzing"
	Approved    TransferState = "Approved"
	FundsPosted TransferState = "FundsPosted"
	Completed   TransferState = "Completed"
	Rejected    TransferState = "Rejected"
	Returned    TransferState = "Returned"
)

// validTransitions defines the allowed state machine transitions.
// Do not modify — canonical source is CLAUDE.md.
var validTransitions = map[TransferState][]TransferState{
	Requested:   {Validating},
	Validating:  {Analyzing, Rejected},
	Analyzing:   {Approved, Rejected},
	Approved:    {FundsPosted},
	FundsPosted: {Completed, Returned},
	Completed:   {Returned},
}

// ErrInvalidTransition is returned when the requested from→to is not in validTransitions.
type ErrInvalidTransition struct {
	From TransferState
	To   TransferState
}

func (e *ErrInvalidTransition) Error() string {
	return fmt.Sprintf("SYS_INVALID_TRANSITION: %s → %s is not a valid transition", e.From, e.To)
}

// Code returns the machine-readable error code.
func (e *ErrInvalidTransition) Code() string { return "SYS_INVALID_TRANSITION" }

// isValidTransition returns true iff from→to appears in validTransitions.
func isValidTransition(from, to TransferState) bool {
	targets, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, t := range targets {
		if t == to {
			return true
		}
	}
	return false
}
