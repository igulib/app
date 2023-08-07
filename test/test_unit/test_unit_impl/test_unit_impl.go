package test_unit_impl

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/igulib/app"
	"github.com/igulib/app/test/test_unit"
)

// TestUnitImpl implements test_unit.TestUnit interface.
// Do not construct this object directly,
// use NewTestUnitImpl instead.
type TestUnitImpl struct {
	runner       *app.UnitLifecycleRunner
	availability atomic.Int32

	startParameters test_unit.OpParameters
	pauseParameters test_unit.OpParameters
	quitParameters  test_unit.OpParameters

	// Fields required for dummy API service
	apiChan chan test_unit.ApiRequest

	apiServiceDisabled              bool
	apiServiceQuitRequestChan       chan struct{}
	apiServiceQuitRequestChanClosed atomic.Bool

	apiServiceStarted              atomic.Bool
	apiServiceQuitConfirmationChan chan struct{}

	apiRequestCompletionWaitGroup sync.WaitGroup

	// Not required in a real project, for testing purposes only
	internalGoroutineCounter atomic.Int32
}

// NewTestUnit returns a new
// properly initialized instance of TestUnitImpl.
func NewTestUnit(name string, dummyApiServiceDisabled ...bool) *TestUnitImpl {
	// create UnitLifecycleRunner
	// that handles lifecycle messages for us.
	tu := &TestUnitImpl{
		runner: app.NewUnitLifecycleRunner(name),
	}
	tu.runner.SetOwner(tu) // don't forget to set the runner's owner!

	// do additional initialization if required
	disabled := false
	if len(dummyApiServiceDisabled) > 0 {
		disabled = dummyApiServiceDisabled[0]
	}
	tu.apiServiceDisabled = disabled
	if !disabled {
		tu.apiChan = make(
			chan test_unit.ApiRequest,
			test_unit.DefaultApiChannelBufferSize)
		tu.apiServiceQuitRequestChan = make(chan struct{})
		tu.apiServiceQuitConfirmationChan = make(chan struct{})
	}
	tu.ResetDefaults()
	return tu
}

// Implement app.IUnit interface
func (tu *TestUnitImpl) Runner() *app.UnitLifecycleRunner {
	return tu.runner
}

func (tu *TestUnitImpl) UnitAvailability() app.UnitAvailability {
	return app.UnitAvailability(tu.availability.Load())
}

func (tu *TestUnitImpl) StartUnit() app.UnitOperationResult {
	if !tu.apiServiceDisabled {
		// Start service only if it is not running yet
		swapped := tu.apiServiceStarted.CompareAndSwap(false, true)
		if swapped {
			go tu.apiService()
		}
	}

	params := &tu.startParameters
	availabilityOnSuccess := app.UAvailable
	r := tu.finalizeLifecycleOp(params, availabilityOnSuccess)
	if r.OK {
		StartOrderTracker.Add(tu.runner.Name())
	}
	return r
}

func (tu *TestUnitImpl) PauseUnit() app.UnitOperationResult {

	params := &tu.pauseParameters
	availabilityOnSuccess := app.UTemporarilyUnavailable

	r := tu.finalizeLifecycleOp(params, availabilityOnSuccess)

	if !params.SimulateFailure {
		// Wait for all ongoing API requests to complete.
		tu.apiRequestCompletionWaitGroup.Wait()
	}

	if r.OK {
		PauseOrderTracker.Add(tu.runner.Name())
	}

	return r
}

func (tu *TestUnitImpl) QuitUnit() app.UnitOperationResult {
	if !tu.apiServiceDisabled {
		swapped := tu.apiServiceQuitRequestChanClosed.CompareAndSwap(false, true)
		if swapped {
			close(tu.apiServiceQuitRequestChan)
			// Wait until apiService goroutine exits
			<-tu.apiServiceQuitConfirmationChan
		}
	}

	params := &tu.quitParameters
	availabilityOnSuccess := app.UNotAvailable
	r := tu.finalizeLifecycleOp(params, availabilityOnSuccess)
	if r.OK {
		QuitOrderTracker.Add(tu.runner.Name())
	}
	return r
}

func (tu *TestUnitImpl) finalizeLifecycleOp(
	params *test_unit.OpParameters,
	availabilityOnSuccess app.UnitAvailability,
) app.UnitOperationResult {
	if params.PeriodMillis < 0 {
		params.PeriodMillis = test_unit.DefaultStartPeriodMillis
	}
	time.Sleep(time.Duration(params.PeriodMillis) * time.Millisecond)
	ok := !params.SimulateFailure
	if ok { // Change availability only if operation succeeds
		tu.setAvailability(availabilityOnSuccess)
	}
	var err error
	if params.SimulateCollateralError {
		err = test_unit.ErrDummy
	}

	r := app.UnitOperationResult{
		OK:              ok,
		CollateralError: err,
	}
	return r
}

// Implement other methods of TestUnit interface
func (tu *TestUnitImpl) setAvailability(a app.UnitAvailability) {
	tu.availability.Store(int32(a))
}

func (tu *TestUnitImpl) SetStartParameters(p test_unit.OpParameters) {
	tu.startParameters = p
}

func (tu *TestUnitImpl) SetPauseParameters(p test_unit.OpParameters) {
	tu.pauseParameters = p
}

func (tu *TestUnitImpl) SetQuitParameters(p test_unit.OpParameters) {
	tu.quitParameters = p
}

func (tu *TestUnitImpl) ResetDefaults() {
	tu.startParameters = test_unit.OpParameters{
		PeriodMillis: test_unit.DefaultStartPeriodMillis,
	}

	tu.pauseParameters = test_unit.OpParameters{
		PeriodMillis: test_unit.DefaultPausePeriodMillis,
	}

	tu.quitParameters = test_unit.OpParameters{
		PeriodMillis: test_unit.DefaultQuitPeriodMillis,
	}
}

func (tu *TestUnitImpl) SendApiRequest(
	msg string,
	simulateDelayMillis int64,
) (chan test_unit.ApiResponse, error) {
	if tu.UnitAvailability() != app.UAvailable {
		return nil, test_unit.ErrNotAvailable
	}
	responseChan := make(chan test_unit.ApiResponse)
	req := test_unit.ApiRequest{
		Payload:             msg,
		ResponseDelayMillis: simulateDelayMillis,
		ResponseChan:        responseChan,
	}
	tu.apiChan <- req
	return responseChan, nil
}

func (tu *TestUnitImpl) WaitForApiResponse(
	ch chan test_unit.ApiResponse, timeoutMillis ...int64) (string, error) {

	var timeout int64 = test_unit.DefaultApiResponseTimeout
	if len(timeoutMillis) > 0 {
		timeout = timeoutMillis[0]
	}
	timer := time.NewTimer(time.Duration(timeout) * time.Millisecond)
	select {
	case msg := <-ch:
		timer.Stop()
		return msg.Msg, msg.Error
	case <-timer.C:
		return "", app.ErrTimedOut
	}
}

// apiService is a dummy API service provided by this unit.
func (tu *TestUnitImpl) apiService() {
	tu.internalGoroutineCounter.Add(1)
	defer func() {
		tu.internalGoroutineCounter.Add(-1)
		swapped := tu.apiServiceStarted.CompareAndSwap(true, false)
		if swapped {
			close(tu.apiServiceQuitConfirmationChan)
		}
	}()
	for {
		select {
		case msg := <-tu.apiChan:
			// Process each request in a separate goroutine
			tu.internalGoroutineCounter.Add(1)
			tu.apiRequestCompletionWaitGroup.Add(1)
			go func() {
				defer func() {
					tu.internalGoroutineCounter.Add(-1)
					tu.apiRequestCompletionWaitGroup.Done()
				}()
				if msg.ResponseDelayMillis > 0 {
					time.Sleep(time.Duration(msg.ResponseDelayMillis) * time.Millisecond)
				}
				// Simply echo the message back.
				// We could also check app.IsShuttingDown()
				// to determine a better processing strategy
				// if required.
				msg.ResponseChan <- test_unit.ApiResponse{
					Error: nil,
					Msg:   msg.Payload,
				}
			}()

		case <-tu.apiServiceQuitRequestChan:
			return
		}
	}
}

func (tu *TestUnitImpl) GoroutineCount() int32 {
	return tu.internalGoroutineCounter.Load()
}

type operationOrderTracker struct {
	lock sync.Mutex
	list []string
}

func (t *operationOrderTracker) Reset() {
	t.lock.Lock()
	t.list = make([]string, 0, 100)
	t.lock.Unlock()
}

func (t *operationOrderTracker) Add(name string) {
	t.lock.Lock()
	t.list = append(t.list, name)
	t.lock.Unlock()
}

func (t *operationOrderTracker) Get() []string {
	t.lock.Lock()
	defer t.lock.Unlock()
	return t.list
}

var (
	StartOrderTracker operationOrderTracker
	PauseOrderTracker operationOrderTracker
	QuitOrderTracker  operationOrderTracker
)

func ResetTrackers() {
	StartOrderTracker.Reset()
	PauseOrderTracker.Reset()
	QuitOrderTracker.Reset()
}

func init() {
	ResetTrackers()
}
