package test_unit

import (
	"errors"

	"github.com/igulib/app"
)

// Defaults and Errors are made public
// as they may be useful for the clients.

// Defaults
const (
	DefaultStartPeriodMillis = 30
	DefaultPausePeriodMillis = 30
	DefaultQuitPeriodMillis  = 30

	DefaultApiChannelBufferSize = 3
	DefaultApiResponseTimeout   = 100
)

// Errors
var (
	ErrNotAvailable = errors.New("not available")
	ErrDummy        = errors.New("dummy error")
)

// TestUnit is a public interface, the "facade" of TestUnitImpl
// that can be imported from any package of your app.
// The facade pattern is used to eliminate potential
// circular dependency problems.
type TestUnit interface {
	app.IUnit
	ResetDefaults()
	SetStartParameters(p OpParameters)
	SetPauseParameters(p OpParameters)
	SetQuitParameters(p OpParameters)

	// A unit in real world may or may not implement API requests via channels.
	// This method demonstrates one way to do it.
	SendApiRequest(msg string, simulateDelayMillis int64) (chan ApiResponse, error)
	WaitForApiResponse(ch chan ApiResponse, timeoutMillis ...int64) (string, error)

	// GoroutineCount returns internal goroutine count.
	// It is useful to prove that there is no goroutine leakage
	// after this unit has been quit.
	GoroutineCount() int32
}

type OpParameters struct {
	// PeriodMillis is the simulated duration of the operation in milliseconds.
	// If negative, the default operation period will be used.
	PeriodMillis int64

	// If true, operation failure will be simulated.
	SimulateFailure bool

	// If true, ErrDummy will be returned as a collateral error.
	SimulateCollateralError bool
}

type ApiRequest struct {
	Payload             string
	ResponseDelayMillis int64
	ResponseChan        chan ApiResponse
}

type ApiResponse struct {
	Error error
	Msg   string
}
