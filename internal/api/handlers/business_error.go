package handlers

import (
	"errors"
	"fmt"
)

// BusinessError is a typed error that carries an HTTP status code.
// Handlers return it from within transaction callbacks; the outer
// error-handling layer uses businessHTTPStatus to map it to the
// correct Fiber response status.
type BusinessError struct {
	HTTPStatus int
	Msg        string
}

func (e *BusinessError) Error() string {
	return e.Msg
}

// newBusinessError creates a *BusinessError with the given HTTP status and message.
func newBusinessError(status int, msg string) *BusinessError {
	return &BusinessError{HTTPStatus: status, Msg: msg}
}

// businessHTTPStatus inspects err. If err wraps a *BusinessError it
// returns the embedded status; otherwise it returns 0 (caller should
// fall through to its default handling).
func businessHTTPStatus(err error) int {
	var be *BusinessError
	if errors.As(err, &be) {
		return be.HTTPStatus
	}
	return 0
}

// ---------------------------------------------------------------------------
// Convenience constructors — one per canonical error used across handlers.
// Centralising the messages here means a single place to change wording.
// ---------------------------------------------------------------------------

// --- Sales contract errors ---

var (
	ErrSalesUnitHasActiveContract   = newBusinessError(409, "unit already has an active sales contract")
	ErrSalesNotDraft                = newBusinessError(409, "can only update draft contracts")
	ErrSalesActivateNotDraft        = newBusinessError(409, "can only activate draft contracts")
	ErrSalesActivateNoScheduleLines = newBusinessError(400, "cannot activate contract with no schedule lines")
	ErrSalesReceivableMismatch      = newBusinessError(409, "schedule line receivable does not match line")
	ErrSalesCancelInvalidStatus     = newBusinessError(409, "invalid contract status for cancellation")
	ErrSalesTerminateNotActive      = newBusinessError(409, "can only terminate active contracts")
	ErrSalesCompleteNotActive       = newBusinessError(409, "can only complete active contracts")
	ErrSalesCompleteNoSchedule      = newBusinessError(400, "contract has no schedule lines")
	ErrSalesCompleteNoReceivable    = newBusinessError(400, "schedule line has no linked receivable")
	ErrSalesCompleteReceivableOpen  = newBusinessError(400, "schedule line receivable not settled")
	ErrSalesCompleteOutstanding     = newBusinessError(400, fmt.Sprintf("schedule line receivable has outstanding balance"))
	ErrSalesDefaultNotActive        = newBusinessError(409, "can only mark active contracts as defaulted")
	ErrSalesScheduleNotDraft        = newBusinessError(409, "can only generate schedule for draft contracts")
	ErrSalesScheduleExists          = newBusinessError(409, "schedule already exists for this contract")
	ErrSalesScheduleHasReceivables  = newBusinessError(409, "schedule has receivables and cannot be regenerated")
	ErrSalesAddLineNotDraft         = newBusinessError(409, "can only add schedule lines to draft contracts")
	ErrSalesUpdateLineBadStatus     = newBusinessError(409, "can only update schedule lines for draft or active contracts")
	ErrSalesLineReceivablePaid      = newBusinessError(409, "schedule line receivable already paid or partially paid")
	ErrSalesLinePastDue             = newBusinessError(409, "can only restructure future schedule lines")

	// --- Lease contract errors ---

	ErrLeaseNotDraft           = newBusinessError(409, "can only edit draft lease contracts")
	ErrLeaseActivateNotDraft   = newBusinessError(409, "can only activate draft lease contracts")
	ErrLeaseUnitHasActive      = newBusinessError(409, "unit already has an active lease contract")
	ErrLeaseGenerateNotActive  = newBusinessError(409, "can only generate bills for active leases")
	ErrLeaseTerminateNotActive = newBusinessError(409, "can only terminate active lease contracts")
	ErrLeaseRenewNotActive     = newBusinessError(409, "can only renew active lease contracts")

	// --- Lease bill errors ---

	ErrLeaseBillIssueNotDraft     = newBusinessError(409, "can only issue draft lease bills")
	ErrLeaseBillMissingReceivable = newBusinessError(409, "issued lease bill is missing linked receivable")

	// --- Reservation errors ---

	ErrReservationUnitActive = newBusinessError(409, "unit already has an active reservation")
	ErrReservationNotActive  = newBusinessError(409, "reservation is not active")

	// --- Ownership transfer errors ---

	ErrTransferContractNotActive = newBusinessError(409, "can only request transfer for active contracts")
	ErrTransferFromPartyInvalid  = newBusinessError(400, "from_party_id is not an active party on this contract")

	// --- Credit balance errors ---

	ErrCreditBalanceNotAvailable    = newBusinessError(400, "credit balance not available")
	ErrCreditBalanceExceeds         = newBusinessError(400, "amount exceeds credit balance remaining")
	ErrCreditReceivableNotOpen      = newBusinessError(400, "receivable not open or partially paid")
	ErrCreditExceedsOutstanding     = newBusinessError(400, "amount exceeds receivable outstanding amount")
)

// voidBillStatusError creates a 409 for "cannot void lease bill in status X".
func voidBillStatusError(status string) *BusinessError {
	return newBusinessError(409, fmt.Sprintf("cannot void lease bill in status %s", status))
}

// voidBillLinkedReceivableError creates a 409 for "cannot void lease bill: linked receivable is X".
func voidBillLinkedReceivableError(status string) *BusinessError {
	return newBusinessError(409, fmt.Sprintf("cannot void lease bill: linked receivable is %s", status))
}
