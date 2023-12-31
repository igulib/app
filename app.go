package app

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Defaults

var (
	DefaultUnitStartPeriodMillis int64 = 5000
	DefaultUnitPausePeriodMillis int64 = 5000
	DefaultUnitQuitPeriodMillis  int64 = 5000

	DefaultUnitLifecycleChannelBufferSize = 1
)

// UnitAvailability describes availability of the unit's API.
type UnitAvailability int32

const (
	// Unit's API is permanently not available.
	UNotAvailable UnitAvailability = iota

	// Unit's API is temporarily unavailable.
	UTemporarilyUnavailable

	// Only part of Unit's API available.
	UPartiallyAvailable

	// Unit's API available.
	UAvailable
)

// Errors
var (
	ErrUnitAlreadyExists        = errors.New("unit already exists")
	ErrUnitHasUnexpectedState   = errors.New("unit has unexpected state")
	ErrUnitNotFound             = errors.New("unit not found")
	ErrStartupSchemeNotDefined  = errors.New("startup scheme not defined")
	ErrShutdownSchemeNotDefined = errors.New("shutdown scheme not defined")
	ErrOperationFailed          = errors.New("operation failed")
	ErrBusy                     = errors.New("busy (previous operation not complete)")
	ErrSkippedByUnitManager     = errors.New("skipped by UnitManager")
	ErrTimedOut                 = errors.New("timed out")
	ErrShuttingDown             = errors.New("already shutting down")

	// ErrOperationIdMismatch means that WaitUntilComplete method
	// has been waiting for wrong operation. This may be a sign of a race condition
	// when you call UnitManager methods from different goroutines without
	// proper synchronization.
	ErrOperationIdMismatch = errors.New("operation ID mismatch")
)

// GlobalShutdownChan signals the application shutdown after it is closed
// by the InitiateShutdown function.
// You can wait for the closure of this channel from any goroutine.
// Do NOT close it manually with 'close' function, use app.InitiateShutdown instead.
var GlobalShutdownChan = make(chan struct{})
var globalShutdownChanClosed atomic.Bool

// InitiateShutdown initiates the global application shutdown.
// This function doesn't block, it is thread-safe and can be called from any goroutine.
//
// Returns:
// nil if it succeeds or error otherwise.
func InitiateShutdown() error {
	swapped := globalShutdownChanClosed.CompareAndSwap(false, true)
	if swapped {
		close(GlobalShutdownChan)
	}
	return nil
}

// IsShuttingDown returns true if the app is currently shutting down.
// It can be used e.g. by a goroutine to plan incoming request processing strategy,
// even though better option would be to use unit's shutdown mechanism.
// Also consider using GlobalShutdownChan in the 'select' statements instead of this function.
//
// This function doesn't block, it is thread-safe and can be called from any goroutine.
func IsShuttingDown() bool {
	return globalShutdownChanClosed.Load()
}

// WaitUntilAppShutdownInitiated blocks until global app shutdown is initiated.
//
// This function is thread-safe and can be called from any goroutine.
func WaitUntilAppShutdownInitiated() {
	<-GlobalShutdownChan
}

var (
	sysSignalChan              chan os.Signal
	sysSignalCancelChan        chan struct{}
	sysSignalCancelConfirmChan chan struct{}
	sysSignalOpLock            sync.Mutex
	sysSignalsSetUpDone        bool
)

// ListenToOSSignals enables app to intercept
// OS signals SIGINT and/or SIGTERM to initiate graceful shutdown
// (SIGKILL can't be intercepted by a user application and thus is not listed as a parameter).
//
// This function doesn't block and is thread-safe.
// Use StopOSSignalListening to revert the effect of calling this function if required.
//
// Example:
//
//	func main() {
//		fmt.Println("== app started ==")
//		// Start app tasks here in separate goroutines...
//		app.ListenToOSSignals(true, true)
//		app.WaitUntilAppShutdownInitiated()  // blocks until Ctrl+C pressed
//		// Do graceful shutdown here...
//		fmt.Println("== app exited ==")
//	}
func ListenToOSSignals(interceptSIGINT, interceptSIGTERM bool) {
	if !(interceptSIGINT || interceptSIGTERM) {
		return
	}
	sysSignalOpLock.Lock()
	defer sysSignalOpLock.Unlock()

	if IsShuttingDown() {
		return
	}

	if sysSignalsSetUpDone {
		return
	}

	signals := make([]os.Signal, 0, 2)
	if interceptSIGINT {
		signals = append(signals, syscall.SIGINT)
	}
	if interceptSIGTERM {
		signals = append(signals, syscall.SIGTERM)
	}
	sysSignalChan = make(chan os.Signal, 1)
	sysSignalCancelChan = make(chan struct{})
	sysSignalCancelConfirmChan = make(chan struct{})
	go func() {
		select {
		case <-sysSignalChan:
			_ = InitiateShutdown()
		case <-sysSignalCancelChan:
			signal.Stop(sysSignalChan)
			sysSignalsSetUpDone = false
			close(sysSignalCancelConfirmChan)
		}
	}()

	signal.Notify(sysSignalChan, signals...)
	sysSignalsSetUpDone = true
}

// StopOSSignalListening cancels the effect of
// previously called ListenToOSSignals
// and disconnects system signals from global app shutdown mechanism.
//
// This function doesn't block and is thread-safe.
// It is allowed to call ListenToOSSignals again after calling this function.
func StopOSSignalListening() {
	sysSignalOpLock.Lock()
	defer sysSignalOpLock.Unlock()

	if IsShuttingDown() {
		return
	}

	if !sysSignalsSetUpDone {
		return
	}

	close(sysSignalCancelChan)
	<-sysSignalCancelConfirmChan
}

// M is a global UnitManager for your app that is created automatically.
// In most cases you don't have to create your own UnitManagers.
var M = NewUnitManager()

// IUnit defines an interface to be implemented by a custom Unit.
type IUnit interface {
	UnitRunner() *UnitLifecycleRunner
	UnitStart() UnitOperationResult
	UnitPause() UnitOperationResult
	UnitQuit() UnitOperationResult
	UnitAvailability() UnitAvailability
}

// MultiUnitOperationConfig defines the configuration
// of a single operation on a set of units.
// Used by UnitManager startup and shutdown schemes.
type MultiUnitOperationConfig struct {

	// UnitNames lists units on which the operation is to be performed.
	UnitNames []string

	// StartTimeoutMillis defines the timeout in milliseconds
	// for the start operation for the whole unit set
	// (and each unit as well because within one set they start in parallel).
	// If this value is zero, the DefaultUnitStartPeriodMillis is used.
	StartTimeoutMillis int64

	// PauseTimeoutMillis defines the timeout in milliseconds
	// for the pause operation for the whole unit set
	// (and each unit as well because within one set they pause in parallel).
	// If this value is zero, the DefaultUnitPausePeriodMillis is used.
	PauseTimeoutMillis int64

	// QuitTimeoutMillis defines the timeout in milliseconds
	// for the quit operation for the whole unit set
	// (and each unit as well because within one set they quit in parallel).
	// If this value is zero, the DefaultUnitQuitPeriodMillis is used.
	QuitTimeoutMillis int64
}

type UnitOperationResult struct {
	// OK indicates overall operation success or failure.
	OK bool

	// CollateralError is an error that could occur during operation
	// but doesn't necessarily lead to the operation failure.
	CollateralError error
}

type UnitManagerOperationResult struct {
	OpName string

	// OpId uniquely identifies the operation.
	OpId int64

	// OK is true if all units completed the operation successfully.
	OK bool

	// CollateralErrors is true if at least one unit encountered
	// a collateral error during operation.
	CollateralErrors bool

	ResultMap map[string]UnitOperationResult
}

type UnitManager struct {
	// opLock protects a series of
	// UnitManager operations.
	// Each UnitManager method is thread-safe on its own,
	// but to ensure that the sequence of method calls is not interrupted
	// from another goroutine, opLock should be acquired first.
	opLock sync.Mutex

	startupScheme  []MultiUnitOperationConfig
	shutdownScheme []MultiUnitOperationConfig
	units          map[string]IUnit

	currentOpType atomic.Int32

	// Channel to be closed when current operation completes.
	completionReportChannel     chan struct{}
	completionReportChannelOpen atomic.Bool
	lastResult                  UnitManagerOperationResult
	currentOpId                 int64
}

func NewUnitManager() *UnitManager {
	um := &UnitManager{
		units: make(map[string]IUnit),
	}

	um.completionReportChannel = make(chan struct{})

	// Initially completionReportChannel is closed
	// so that WaitUntilComplete method doesn't block
	// if accidentally called.
	close(um.completionReportChannel)
	return um
}

// GetUnit returns a reference to the Unit with the specified name.
// The reference is guaranteed to remain valid
// as long as the UnitManager itself is valid because,
// once the Unit is added to the UnitManager, it can't be removed.
func (um *UnitManager) GetUnit(name string) (IUnit, error) {
	um.opLock.Lock()
	defer um.opLock.Unlock()

	swapped := um.currentOpType.CompareAndSwap(umcoIdle, umcoGetInfo)
	if !swapped {
		return nil, ErrBusy
	}
	unit, ok := um.units[name]
	um.currentOpType.Store(umcoIdle)
	if !ok {
		return nil, ErrUnitNotFound
	}
	return unit, nil
}

// ListUnitStates lists all units previously added to UnitManager
// and their current states.
func (um *UnitManager) ListUnitStates() (map[string]int32, error) {
	um.opLock.Lock()
	defer um.opLock.Unlock()

	swapped := um.currentOpType.CompareAndSwap(umcoIdle, umcoGetInfo)
	if !swapped {
		return nil, ErrBusy
	}
	m := make(map[string]int32, len(um.units))
	for name, unit := range um.units {
		m[name] = unit.UnitRunner().State()
	}
	um.currentOpType.Store(umcoIdle)
	return m, nil
}

// getUniqueUnitNames returns names from 'nameList' that are not present in um.units.
func (um *UnitManager) getUniqueUnitNames(nameList []string) []string {
	unitsTotal := len(um.units)
	absentList := make([]string, 0, unitsTotal)
	for _, name := range nameList {
		_, ok := um.units[name]
		if !ok {
			absentList = append(absentList, name)
		}
	}
	return absentList
}

// SetOperationScheme sets the operation scheme that defines both
// unit start and pause orders and respective timeouts.
// The pause order will be the reverse of the start order.
// If there is a need of a custom pause order, use SetCustomPauseScheme
// method in addition to this.
func (um *UnitManager) SetOperationScheme(scheme []MultiUnitOperationConfig) error {
	um.opLock.Lock()
	defer um.opLock.Unlock()

	swapped := um.currentOpType.CompareAndSwap(umcoIdle, umcoModifyScheme)
	if !swapped {
		return ErrBusy
	}
	defer func() {
		um.currentOpType.CompareAndSwap(umcoModifyScheme, umcoIdle)
	}()

	um.startupScheme = scheme
	// Check that all names from the scheme are present in UnitManager.
	unitsTotal := len(um.units)
	names := make([]string, 0, unitsTotal)
	for _, layer := range scheme {
		names = append(names, layer.UnitNames...)
	}
	absentList := um.getUniqueUnitNames(names)
	if len(absentList) == 0 {
		return nil
	}
	return fmt.Errorf("unit(s) not found: %v: %w", absentList, ErrUnitNotFound)
}

// SetCustomPauseScheme sets the shutdown scheme for UnitManager.
// If not set, the shutdown order will be reverse to startup order.
func (um *UnitManager) SetCustomPauseScheme(scheme []MultiUnitOperationConfig) error {
	um.opLock.Lock()
	defer um.opLock.Unlock()

	swapped := um.currentOpType.CompareAndSwap(umcoIdle, umcoModifyScheme)
	if !swapped {
		return ErrBusy
	}
	defer func() {
		um.currentOpType.CompareAndSwap(umcoModifyScheme, umcoIdle)
	}()

	um.shutdownScheme = scheme

	// Check that all names from the scheme are present in UnitManager.
	unitsTotal := len(um.units)
	names := make([]string, 0, unitsTotal)
	for _, layer := range scheme {
		names = append(names, layer.UnitNames...)
	}
	absentList := um.getUniqueUnitNames(names)
	if len(absentList) == 0 {
		return nil
	}
	return fmt.Errorf("unit(s) not found: %v: %w", absentList, ErrUnitNotFound)
}

// AddUnit is thread-safe.
func (um *UnitManager) AddUnit(unit IUnit) error {
	// Lock the asynchronous operation series
	um.opLock.Lock()
	defer um.opLock.Unlock()

	swapped := um.currentOpType.CompareAndSwap(umcoIdle, umcoInit)
	if !swapped {
		return ErrBusy
	}

	defer func() {
		swapped := um.currentOpType.CompareAndSwap(umcoInit, umcoIdle)
		if !swapped {
			panic("igulib/app.UnitManager.AddUnit operation atomicity violated.")
		}
	}()

	name := unit.UnitRunner().name
	_, alreadyExists := um.units[name]
	if alreadyExists {
		// No need to reset um.currentOperation to umcoIdle here
		// as that is done in the deferred function above.
		return ErrUnitAlreadyExists
	}

	swapped = unit.UnitRunner().state.CompareAndSwap(STNotInitialized, STInitializing)
	if !swapped {
		return ErrUnitHasUnexpectedState // normally can't happen
	}

	um.units[name] = unit

	// Start unit's lifecycle message loop
	err := unit.UnitRunner().prepareNextAsyncOperation(UmcoInit, um.currentOpId)
	if err != nil {
		return ErrBusy // normally can't happen
	}
	go unit.UnitRunner().run()
	unit.UnitRunner().runnerLock.Lock()
	doneChan := unit.UnitRunner().runnerOpDoneChannel
	unit.UnitRunner().runnerLock.Unlock()
	<-doneChan
	if unit.UnitRunner().state.Load() != STPaused {
		return ErrOperationFailed // normally can't happen
	}
	return nil
}

func (um *UnitManager) getStartupSchemeUnitNames() []string {
	unitNames := make([]string, 0, len(um.units))
	if um.startupScheme == nil {
		return unitNames
	}
	for _, layer := range um.startupScheme {
		unitNames = append(unitNames, layer.UnitNames...)
	}
	return unitNames
}

func (um *UnitManager) getShutdownSchemeUnitNames() []string {
	unitNames := make([]string, 0, len(um.units))
	if um.shutdownScheme != nil {
		for _, layer := range um.shutdownScheme {
			unitNames = append(unitNames, layer.UnitNames...)
		}
		return unitNames
	}
	return unitNames
}

// Prepare internal state for next async operation.
// unitNames is a list of units on which the operation will be performed.
func (um *UnitManager) prepareNextManagerAsyncOperation(
	opName string, unitNames []string) error {
	swapped := um.completionReportChannelOpen.CompareAndSwap(false, true)
	if !swapped {
		return ErrBusy
	}

	um.currentOpId++

	um.completionReportChannel = make(chan struct{})
	um.lastResult = UnitManagerOperationResult{
		OpName: opName,
		ResultMap: func() map[string]UnitOperationResult {
			m := make(map[string]UnitOperationResult, len(unitNames))
			for _, unitName := range unitNames {
				m[unitName] = UnitOperationResult{}
			}
			return m
		}(),
	}
	return nil
}

// Find MultiUnitOperationConfig with specified unitName.
func (um *UnitManager) findUnitSchemeLayer(
	unitName string, layers []MultiUnitOperationConfig) *MultiUnitOperationConfig {
	for _, layer := range layers {
		for _, name := range layer.UnitNames {
			if name == unitName {
				return &layer
			}
		}
	}
	return nil
}

// startAsync asynchronously starts the specified unit.
// Use WaitUntilComplete method to wait until operation completes
// and get the result.
func (um *UnitManager) startAsync(unitName string, timeoutMillis ...int64) (int64, error) {
	// Choose correct timeout:
	var timeout int64 = 0
	// First try the supplied timeout
	if len(timeoutMillis) > 0 {
		if timeoutMillis[0] > 0 {
			timeout = timeoutMillis[0]
		}
	}
	// Second try one of the scheme timeouts
	if timeout == 0 {
		schemes := [][]MultiUnitOperationConfig{um.startupScheme, um.shutdownScheme}
		for _, scheme := range schemes {
			if scheme == nil {
				continue
			}
			layer := um.findUnitSchemeLayer(unitName, scheme)
			if layer != nil {
				if layer.StartTimeoutMillis > 0 {
					timeout = layer.StartTimeoutMillis
					break
				}
			}
		}
	}
	// Or escape to the default value
	if timeout == 0 {
		timeout = DefaultUnitStartPeriodMillis
	}

	return um.initiateSingleOperation(
		unitName,
		umcoStart,
		STPaused,
		STStarting,
		STStarted,
		lcmStart,
		timeout)
}

// Start starts the unit and blocks until complete.
// Output parameters:
//
// UnitOperationResult - unit internal error if any;
// error - UnitManager error.
func (um *UnitManager) Start(unitName string, timeoutMillis ...int64) (
	UnitOperationResult, error) {
	um.opLock.Lock()
	defer um.opLock.Unlock()

	id, err := um.startAsync(unitName, timeoutMillis...)
	if err != nil {
		return UnitOperationResult{}, err
	}
	r := um.WaitUntilComplete()
	if r.OpId != id {
		return r.ResultMap[unitName], ErrOperationIdMismatch
	}
	if !r.OK {
		return r.ResultMap[unitName], ErrOperationFailed
	}
	return r.ResultMap[unitName], nil
}

// pauseAsync asynchronously pauses the specified unit.
// Use WaitUntilComplete method to wait until operation completes
// and get the result.
func (um *UnitManager) pauseAsync(unitName string, timeoutMillis ...int64) (int64, error) {
	// Choose correct timeout:
	var timeout int64 = 0
	// First try the supplied timeout
	if len(timeoutMillis) > 0 {
		if timeoutMillis[0] > 0 {
			timeout = timeoutMillis[0]
		}
	}
	// Second try one of the scheme timeouts
	if timeout == 0 {
		schemes := [][]MultiUnitOperationConfig{um.shutdownScheme, um.startupScheme}
		for _, scheme := range schemes {
			if scheme == nil {
				continue
			}
			layer := um.findUnitSchemeLayer(unitName, scheme)
			if layer != nil {
				if layer.PauseTimeoutMillis > 0 {
					timeout = layer.PauseTimeoutMillis
					break
				}
			}
		}
	}
	// Or escape to the default value
	if timeout == 0 {
		timeout = DefaultUnitPausePeriodMillis
	}

	return um.initiateSingleOperation(
		unitName,
		umcoPause,
		STStarted,
		STPausing,
		STPaused,
		lcmPause,
		timeout)
}

// Pause pauses the unit and blocks until complete.
// Output parameters:
//
// UnitOperationResult - unit internal error if any;
// error - UnitManager error.
func (um *UnitManager) Pause(unitName string, timeoutMillis ...int64) (
	UnitOperationResult, error) {
	um.opLock.Lock()
	defer um.opLock.Unlock()

	id, err := um.pauseAsync(unitName, timeoutMillis...)
	if err != nil {
		return UnitOperationResult{}, err
	}
	r := um.WaitUntilComplete()
	if r.OpId != id {
		return r.ResultMap[unitName], ErrOperationIdMismatch
	}
	if !r.OK {
		return r.ResultMap[unitName], ErrOperationFailed
	}
	return r.ResultMap[unitName], nil
}

// quitAsync asynchronously quits the specified unit.
// Use WaitUntilComplete method to wait until operation completes
// and get the result.
func (um *UnitManager) quitAsync(unitName string, timeoutMillis ...int64) (int64, error) {
	// Choose correct timeout:
	var timeout int64 = 0
	// First try the supplied timeout
	if len(timeoutMillis) > 0 {
		if timeoutMillis[0] > 0 {
			timeout = timeoutMillis[0]
		}
	}
	// Second try one of the scheme timeouts
	if timeout == 0 {
		schemes := [][]MultiUnitOperationConfig{um.shutdownScheme, um.startupScheme}
		for _, scheme := range schemes {
			if scheme == nil {
				continue
			}
			layer := um.findUnitSchemeLayer(unitName, scheme)
			if layer != nil {
				if layer.QuitTimeoutMillis > 0 {
					timeout = layer.QuitTimeoutMillis
					break
				}
			}
		}
	}
	// Or escape to the default value
	if timeout == 0 {
		timeout = DefaultUnitQuitPeriodMillis
	}

	return um.initiateSingleOperation(
		unitName,
		umcoQuit,
		STPaused,
		STQuitting,
		STQuit,
		lcmQuit,
		timeout)
}

// Quit quits the unit and blocks until complete.
// Output parameters:
//
// UnitOperationResult - unit internal error if any;
// error - UnitManager error.
func (um *UnitManager) Quit(unitName string, timeoutMillis ...int64) (
	UnitOperationResult, error) {
	um.opLock.Lock()
	defer um.opLock.Unlock()

	id, err := um.quitAsync(unitName, timeoutMillis...)
	if err != nil {
		return UnitOperationResult{}, err
	}
	r := um.WaitUntilComplete()
	if r.OpId != id {
		return r.ResultMap[unitName], ErrOperationIdMismatch
	}
	if !r.OK {
		return r.ResultMap[unitName], ErrOperationFailed
	}
	return r.ResultMap[unitName], nil
}

func (um *UnitManager) initiateSingleOperation(
	unitName string,
	currentOp int32,
	iState int32, // initial state
	progressState int32,
	tState int32, // target state
	controlMsg string,
	timeout int64) (int64, error) {

	swapped := um.currentOpType.CompareAndSwap(umcoIdle, currentOp)
	if !swapped {
		return NegativeOpId, ErrBusy
	}

	currentOpName := umcoStrings[currentOp]

	// Find unit
	unit, ok := um.units[unitName]
	if !ok {
		um.currentOpType.CompareAndSwap(currentOp, umcoIdle)
		return NegativeOpId, ErrUnitNotFound
	}

	// Check unit is in correct state
	state := unit.UnitRunner().state.Load()
	if !(state == iState || state == progressState || state == tState) {
		um.currentOpType.CompareAndSwap(currentOp, umcoIdle)
		return NegativeOpId, ErrUnitHasUnexpectedState
	}

	err := um.prepareNextManagerAsyncOperation(currentOpName, []string{unitName})
	if err != nil {
		um.currentOpType.CompareAndSwap(currentOp, umcoIdle)
		return NegativeOpId, ErrBusy
	}

	// Return if unit is already in target state
	if state == tState {
		// Report operation completion.
		swapped := um.completionReportChannelOpen.CompareAndSwap(true, false)
		if swapped {
			// Mark unit operation result as success
			// because the unit's UnitStart won't be executed in this case.
			um.lastResult.ResultMap[unitName] = UnitOperationResult{
				OK: true,
			}
			um.fillInLastResult()
			// Set current operation to umcoIdle.
			um.currentOpType.CompareAndSwap(currentOp, umcoIdle)
			close(um.completionReportChannel)
		}
		return um.currentOpId, nil
	}

	runner := unit.UnitRunner()

	// Renew lastOpName and lastOpId if unit is already in progressState
	// but don't send control message.
	if state == progressState {
		runner.runnerLock.Lock()
		runner.lastRunnerOpName = currentOpName
		runner.lastRunnerOpId = um.currentOpId
		runner.runnerLock.Unlock()
	}

	// Send control message only if unit has completed previous operation
	// and its state is iState
	swapped = runner.state.CompareAndSwap(iState, progressState)
	if swapped {
		err := runner.prepareNextAsyncOperation(currentOpName, um.currentOpId)
		if err == nil {
			runner.lifecycleMsgChannel <- lifecycleMsg{
				msgType: controlMsg,
			}
		}
	}

	// Wait in separate goroutine until operation completes or timeout occurs
	go func() {

		timer := time.NewTimer(time.Duration(timeout * int64(time.Millisecond)))
		runner.runnerLock.Lock()
		doneChan := runner.runnerOpDoneChannel
		runner.runnerLock.Unlock()
		select {
		case <-doneChan:
		case <-timer.C:
			state = runner.state.Load()
			if state == progressState {
				runner.runnerLock.Lock()
				runner.lastRunnerResult.CollateralError = ErrTimedOut
				runner.lastRunnerResult.OK = false
				runner.runnerLock.Unlock()
			}
		}
		timer.Stop()

		// Report operation completion.
		swapped := um.completionReportChannelOpen.CompareAndSwap(true, false)
		if swapped {
			um.fillInLastResult()
			// Set current operation to umcoIdle.
			um.currentOpType.CompareAndSwap(currentOp, umcoIdle)
			close(um.completionReportChannel)
		}
	}()

	return um.currentOpId, nil
}

func (um *UnitManager) startSchemeAsync() (int64, []string, error) {
	currentOp := umcoStart

	// Return immediately if previous operation not complete
	swapped := um.currentOpType.CompareAndSwap(umcoIdle, currentOp)
	if !swapped {
		return NegativeOpId, make([]string, 0), ErrBusy
	}

	currentOpName := UmcoStart
	controlMsg := lcmStart
	iState := STPaused
	progressState := STStarting
	tState := STStarted

	badStateUnits := make([]string, 0, len(um.units))

	if um.startupScheme == nil {
		return NegativeOpId, badStateUnits, ErrStartupSchemeNotDefined
	}

	if len(um.startupScheme) == 0 {
		return NegativeOpId, badStateUnits, ErrStartupSchemeNotDefined
	}

	// Check allowed unit states
	for _, layer := range um.startupScheme {
		for _, unitName := range layer.UnitNames {
			// Unit with name from um.startupScheme is guaranteed to exist
			unit := um.units[unitName]
			state := unit.UnitRunner().state.Load()
			if !(state == iState || state == progressState || state == tState) {
				badStateUnits = append(badStateUnits, unitName)
			}
		}
	}

	const atomicityViolated = "igulib/app.UnitManager.StartSchemeAsync operation atomicity violated. Please report this issue on github."

	if len(badStateUnits) != 0 {
		swapped := um.currentOpType.CompareAndSwap(currentOp, umcoIdle)
		if !swapped {
			panic(atomicityViolated)
		}
		return NegativeOpId, badStateUnits, ErrUnitHasUnexpectedState
	}

	err := um.prepareNextManagerAsyncOperation(
		currentOpName, um.getStartupSchemeUnitNames())
	if err != nil {
		swapped := um.currentOpType.CompareAndSwap(currentOp, umcoIdle)
		if !swapped {
			panic(atomicityViolated)
		}
		return NegativeOpId, badStateUnits, ErrBusy
	}
	go um.performOperationByLayers(
		currentOp,
		controlMsg,
		iState,
		progressState,
		tState,
		um.startupScheme,
	)

	return um.currentOpId, badStateUnits, nil
}

// StartScheme starts scheme synchronously and blocks until it is complete.
func (um *UnitManager) StartScheme() (UnitManagerOperationResult, error) {
	um.opLock.Lock()
	defer um.opLock.Unlock()

	opId, failedUnits, err := um.startSchemeAsync()
	r := UnitManagerOperationResult{
		ResultMap: make(map[string]UnitOperationResult, 0),
	}

	if err != nil {
		for _, failedUnit := range failedUnits {
			r.CollateralErrors = true
			r.OpId = opId
			r.ResultMap[failedUnit] = UnitOperationResult{
				CollateralError: ErrOperationFailed,
			}
		}
		return r, err
	}
	r = um.WaitUntilComplete()
	if r.OpId != opId {
		r.OK = false
	}
	if r.OK {
		return r, nil
	}
	return r, ErrOperationFailed
}

func (um *UnitManager) performOperationByLayers(
	currentOp int32,
	controlMsg string,
	iState int32,
	progressState int32,
	tState int32,
	currentScheme []MultiUnitOperationConfig,
) {
	currentOpName := umcoStrings[currentOp]

	skipRemainingLayers := false
	for _, layer := range currentScheme {
		if skipRemainingLayers { // if previous layer operation failed
			for _, unitName := range layer.UnitNames {
				runner := um.units[unitName].UnitRunner()
				runner.runnerLock.Lock()
				runner.lastRunnerOpName = currentOpName
				runner.lastRunnerOpId = um.currentOpId
				runner.lastRunnerResult.OK = false
				runner.lastRunnerResult.CollateralError = ErrSkippedByUnitManager
				runner.runnerLock.Unlock()
			}
			continue
		}
		for _, unitName := range layer.UnitNames {
			runner := um.units[unitName].UnitRunner()
			state := runner.state.Load()
			// Check if unit is already in the target state
			if state == tState {
				// Renew lastOpName, lastOpId and lastResult
				runner.runnerLock.Lock()
				runner.lastRunnerOpName = currentOpName
				runner.lastRunnerOpId = um.currentOpId
				runner.lastRunnerResult.CollateralError = nil
				runner.lastRunnerResult.OK = true
				runner.runnerLock.Unlock()
			} else if state == progressState {
				// Renew lastOpName and lastOpId if unit is already in progressState
				// but don't send control message.
				runner.runnerLock.Lock()
				runner.lastRunnerOpName = currentOpName
				runner.lastRunnerOpId = um.currentOpId
				runner.runnerLock.Unlock()
			}

			// Send control message only if unit has completed previous operation
			// and its state is iState
			swapped := runner.state.CompareAndSwap(iState, progressState)
			if swapped {
				err := runner.prepareNextAsyncOperation(currentOpName, um.currentOpId)
				if err == nil {
					runner.lifecycleMsgChannel <- lifecycleMsg{
						msgType: controlMsg,
					}
				}
			}
		}

		// Wait until either all tasks in the layer start or timeout occurs.
		bulkCompleteChan := make(chan struct{})
		currentLayer := layer
		go func() {
			for _, unitName := range currentLayer.UnitNames {
				runner := um.units[unitName].UnitRunner()
				runner.runnerLock.Lock()
				unitOpDoneCh := runner.runnerOpDoneChannel
				runner.runnerLock.Unlock()
				<-unitOpDoneCh
			}
			close(bulkCompleteChan)
		}()

		var timeout int64
		if currentOp == umcoStart {
			timeout = currentLayer.StartTimeoutMillis
			if timeout == 0 {
				timeout = DefaultUnitStartPeriodMillis
			}
		} else if currentOp == umcoPause {
			timeout = currentLayer.PauseTimeoutMillis
			if timeout == 0 {
				timeout = DefaultUnitPausePeriodMillis
			}
		}

		timer := time.NewTimer(time.Duration(timeout) * time.Millisecond)
		select {
		case <-bulkCompleteChan:
		case <-timer.C:
			// Check for units that timed out
			for _, unitName := range currentLayer.UnitNames {
				runner := um.units[unitName].UnitRunner()
				state := runner.state.Load()

				if state == progressState {
					runner.runnerLock.Lock()
					runner.lastRunnerResult.CollateralError = ErrTimedOut
					runner.lastRunnerResult.OK = false
					runner.runnerLock.Unlock()
				}
			}
		}
		timer.Stop()

		// Skip remaining layers if current layer operation failed
		// (only for StartScheme).
		if currentOp == umcoStart {
			for _, unitName := range currentLayer.UnitNames {
				if um.units[unitName].UnitRunner().state.Load() != tState {
					skipRemainingLayers = true

				}
			}
		}
	}

	// Report operation completion.
	swapped := um.completionReportChannelOpen.CompareAndSwap(true, false)
	if swapped {
		um.fillInLastResult()
		// Set current operation to umcoIdle.
		um.currentOpType.CompareAndSwap(currentOp, umcoIdle)
		close(um.completionReportChannel)
	}
}

func (um *UnitManager) fillInLastResult() {
	unitNames := make([]string, 0, len(um.lastResult.ResultMap))
	for unitName := range um.lastResult.ResultMap {
		unitNames = append(unitNames, unitName)
	}
	um.lastResult.OK = true
	um.lastResult.CollateralErrors = false
	um.lastResult.OpId = um.currentOpId

	for _, unitName := range unitNames {
		runner := um.units[unitName].UnitRunner()
		runner.runnerLock.Lock()
		// Copy unit result only if opIds match
		if runner.lastRunnerOpId == um.lastResult.OpId {
			um.lastResult.ResultMap[unitName] = runner.lastRunnerResult
		}
		if !um.lastResult.ResultMap[unitName].OK {
			um.lastResult.OK = false
		}
		if um.lastResult.ResultMap[unitName].CollateralError != nil {
			um.lastResult.CollateralErrors = true
		}
		runner.runnerLock.Unlock()
	}
}

// WaitUntilComplete blocks until current operation completes
// and returns the result.
// It is thread-safe, multiple goroutines can wait simultaneously.
func (um *UnitManager) WaitUntilComplete() UnitManagerOperationResult {
	<-um.completionReportChannel
	return um.lastResult
}

func (um *UnitManager) pauseSchemeAsync() (int64, []string, error) {
	// Return immediately if previous operation not complete
	swapped := um.currentOpType.CompareAndSwap(umcoIdle, umcoPause)
	if !swapped {
		return NegativeOpId, make([]string, 0), ErrBusy
	}

	currentOp := umcoPause
	currentOpName := UmcoPause
	controlMsg := lcmPause
	iState := STStarted
	progressState := STPausing
	tState := STPaused

	const atomicityViolated = "igulib/app.UnitManager.PauseSchemeAsync operation atomicity violated. Please report this issue on github."

	badStateUnits := make([]string, 0, len(um.units))

	// Create shutdownScheme if it doesn't exist
	if um.shutdownScheme == nil {
		if um.startupScheme == nil {
			swapped := um.currentOpType.CompareAndSwap(currentOp, umcoIdle)
			if !swapped {
				panic(atomicityViolated)
			}
			return NegativeOpId, badStateUnits, ErrShutdownSchemeNotDefined
		}
		// If shutdown scheme not set, use the reverse of startup scheme
		layersTotal := len(um.startupScheme)
		um.shutdownScheme = make([]MultiUnitOperationConfig, 0, layersTotal)
		for i := layersTotal - 1; i >= 0; i-- {
			um.shutdownScheme = append(um.shutdownScheme, um.startupScheme[i])
		}
	}

	if len(um.shutdownScheme) == 0 {
		return NegativeOpId, badStateUnits, ErrShutdownSchemeNotDefined
	}

	// Check allowed unit states
	for _, layer := range um.shutdownScheme {
		for _, unitName := range layer.UnitNames {
			// Unit with name from um.shutdownScheme is guaranteed to exist
			unit := um.units[unitName]
			state := unit.UnitRunner().state.Load()
			if !(state == iState || state == progressState || state == tState) {
				badStateUnits = append(badStateUnits, unitName)
			}
		}
	}

	if len(badStateUnits) != 0 {
		swapped := um.currentOpType.CompareAndSwap(currentOp, umcoIdle)
		if !swapped {
			panic(atomicityViolated)
		}
		return NegativeOpId, badStateUnits, ErrUnitHasUnexpectedState
	}

	err := um.prepareNextManagerAsyncOperation(
		currentOpName, um.getShutdownSchemeUnitNames())
	if err != nil {
		swapped := um.currentOpType.CompareAndSwap(currentOp, umcoIdle)
		if !swapped {
			panic(atomicityViolated)
		}
		return NegativeOpId, badStateUnits, ErrBusy
	}

	go um.performOperationByLayers(
		currentOp,
		controlMsg,
		iState,
		progressState,
		tState,
		um.shutdownScheme,
	)

	return um.currentOpId, badStateUnits, nil
}

// PauseScheme pauses scheme synchronously and blocks until it is complete.
func (um *UnitManager) PauseScheme() (UnitManagerOperationResult, error) {
	um.opLock.Lock()
	defer um.opLock.Unlock()

	opId, failedUnits, err := um.pauseSchemeAsync()
	r := UnitManagerOperationResult{
		ResultMap: make(map[string]UnitOperationResult, 0),
	}

	if err != nil {
		for _, failedUnit := range failedUnits {
			r.CollateralErrors = true
			r.OpId = opId
			r.ResultMap[failedUnit] = UnitOperationResult{
				CollateralError: ErrOperationFailed,
			}
		}
		return r, err
	}
	r = um.WaitUntilComplete()
	if r.OpId != opId {
		r.OK = false
	}
	if r.OK {
		return r, nil
	}
	return r, ErrOperationFailed
}

// quitAllAsync quits all units asynchronously in parallel.
// Units must be in one of [STPaused, STQuitting, STQuit] states
// in order to complete this operation successfully.
// Names of units that have inappropriate state
// will be reported via the second returned parameter along with error.
func (um *UnitManager) quitAllAsync() (int64, []string, error) {
	currentOpName := UmcoQuit
	controlMsg := lcmQuit
	iState := STPaused
	progressState := STQuitting
	tState := STQuit

	// Return immediately if previous operation not complete
	swapped := um.currentOpType.CompareAndSwap(umcoIdle, umcoQuit)
	if !swapped {
		return NegativeOpId, make([]string, 0), ErrBusy
	}

	// Create two lists: units that have legitimate state and units that have bad state.
	unitsTotal := len(um.units)
	okUnits := make([]string, 0, unitsTotal)
	badStateUnits := make([]string, 0, unitsTotal)
	for name, unit := range um.units {
		state := unit.UnitRunner().state.Load()
		if state == iState || state == progressState || state == tState {
			okUnits = append(okUnits, name)
		} else {
			badStateUnits = append(badStateUnits, name)
		}
	}

	const atomicityViolated = "igulib/app.UnitManager.QuitAllAsync operation atomicity violated. Please report this issue on github."

	err := um.prepareNextManagerAsyncOperation(
		currentOpName, um.getShutdownSchemeUnitNames())
	if err != nil {
		swapped := um.currentOpType.CompareAndSwap(umcoQuit, umcoIdle)
		if !swapped {
			panic(atomicityViolated)
		}
		return NegativeOpId, badStateUnits, ErrBusy
	}

	// Set result for units that have bad state
	for _, unitName := range badStateUnits {
		um.lastResult.ResultMap[unitName] = UnitOperationResult{
			CollateralError: ErrUnitHasUnexpectedState,
		}
	}

	go func() {
		// Send quit message to all units in legitimate state.
		for _, unitName := range okUnits {
			runner := um.units[unitName].UnitRunner()
			state := runner.state.Load()
			// Check if unit is already in the target state
			if state == tState {
				// Renew lastOpName, lastOpId and lastResult
				runner.runnerLock.Lock()
				runner.lastRunnerOpName = currentOpName
				runner.lastRunnerOpId = um.currentOpId
				runner.lastRunnerResult.CollateralError = nil
				runner.lastRunnerResult.OK = true
				runner.runnerLock.Unlock()
			} else if state == progressState {
				// Renew lastOpName and lastOpId if unit is already in progressState
				// but don't send control message.
				runner.runnerLock.Lock()
				runner.lastRunnerOpName = currentOpName
				runner.lastRunnerOpId = um.currentOpId
				runner.runnerLock.Unlock()
			}

			// Send control message only if unit has completed previous operation
			// and its state is iState
			swapped := runner.state.CompareAndSwap(iState, progressState)
			if swapped {
				err := runner.prepareNextAsyncOperation(currentOpName, um.currentOpId)
				if err == nil {
					runner.lifecycleMsgChannel <- lifecycleMsg{
						msgType: controlMsg,
					}
				}
			}
		}

		// Wait until units quit or timeout occurs
		bulkCompleteChan := make(chan struct{})
		go func() {
			for _, unitName := range okUnits {
				runner := um.units[unitName].UnitRunner()
				runner.runnerLock.Lock()
				doneChan := runner.runnerOpDoneChannel
				runner.runnerLock.Unlock()
				<-doneChan
			}
			close(bulkCompleteChan)
		}()

		// Use the greatest quit timeout among all layers.
		var timeout int64 = 0
		scheme := um.shutdownScheme
		if scheme == nil {
			scheme = um.startupScheme
		}

		if scheme == nil { // if neither shutdown nor startup scheme defined
			timeout = DefaultUnitQuitPeriodMillis
		} else {
			for _, layer := range scheme {
				layerQuitTimeout := layer.QuitTimeoutMillis
				if layerQuitTimeout == 0 {
					layerQuitTimeout = DefaultUnitQuitPeriodMillis
				}
				if layerQuitTimeout > timeout {
					timeout = layerQuitTimeout
				}
			}
		}

		timer := time.NewTimer(time.Duration(timeout * int64(time.Millisecond)))
		select {
		case <-bulkCompleteChan:
		case <-timer.C:
			// Check for units that timed out
			for _, unitName := range okUnits {
				runner := um.units[unitName].UnitRunner()
				state := runner.state.Load()
				if state == STQuitting {
					runner.runnerLock.Lock()
					runner.lastRunnerResult.CollateralError = ErrTimedOut
					runner.lastRunnerResult.OK = false
					runner.runnerLock.Unlock()
				}
			}
		}
		timer.Stop()

		// Report operation completion.
		swapped = um.completionReportChannelOpen.CompareAndSwap(true, false)
		if swapped {
			um.fillInLastResult()
			// Set current operation to umcoIdle.
			swapped := um.currentOpType.CompareAndSwap(umcoQuit, umcoIdle)
			if !swapped {
				panic(atomicityViolated)
			}
			close(um.completionReportChannel)
		}
	}()

	if len(badStateUnits) > 0 {
		return um.currentOpId, badStateUnits, ErrUnitHasUnexpectedState
	}

	return um.currentOpId, badStateUnits, nil
}

// QuitAll quits all units of the UnitManager and blocks until it is complete.
func (um *UnitManager) QuitAll() (UnitManagerOperationResult, error) {
	um.opLock.Lock()
	defer um.opLock.Unlock()

	opId, failedUnits, err := um.quitAllAsync()

	// quitAllAsync will try quit those units that are in correct state
	// TODO: test edge cases: empty scheme, all in bad state, all quit.
	r := um.WaitUntilComplete()
	if err != nil {
		for _, failedUnit := range failedUnits {
			r.CollateralErrors = true
			r.OpId = opId
			r.ResultMap[failedUnit] = UnitOperationResult{
				CollateralError: err,
			}
		}
		return r, ErrOperationFailed
	}

	if r.OpId != opId {
		r.OK = false
	}
	if r.OK {
		return r, nil
	}
	return r, ErrOperationFailed
}

// UnitLifecycleRunner runs the lifecycle message loop for each Unit.
type UnitLifecycleRunner struct {
	// Constant fields that do not require synchronization.
	name  string
	owner IUnit
	// Channel for inbound messages from UnitManager
	lifecycleMsgChannel chan lifecycleMsg

	// Fields that are synchronized independently.
	state                   atomic.Int32
	runnerOpDoneChannelOpen atomic.Bool

	runnerLock sync.Mutex

	// Fields that are synchronized by mutex.
	lastRunnerResult UnitOperationResult
	lastRunnerOpName string
	lastRunnerOpId   int64

	// runnerOpDoneChannel is an outbound channel to signal
	// that current operation is complete by unit.
	// A new runnerOpDoneChannel is created for each operation.
	runnerOpDoneChannel chan struct{}
}

func NewUnitLifecycleRunner(name string) *UnitLifecycleRunner {
	r := &UnitLifecycleRunner{
		name:                name,
		lifecycleMsgChannel: make(chan lifecycleMsg, DefaultUnitLifecycleChannelBufferSize),
	}
	return r
}

func (r *UnitLifecycleRunner) SetOwner(owner IUnit) {
	r.owner = owner
}

// State returns current state of the unit. It is not guaranteed that
// the state won't change in the nearest future.
func (r *UnitLifecycleRunner) State() int32 {
	return r.state.Load()
}

// prepareNextAsyncOperation creates a new completionReportChannel
// and sets completionReportChannelOpen to true.
func (r *UnitLifecycleRunner) prepareNextAsyncOperation(opName string, opId int64) error {
	swapped := r.runnerOpDoneChannelOpen.CompareAndSwap(false, true)
	if !swapped {
		return ErrBusy
	}
	r.runnerLock.Lock()
	r.runnerOpDoneChannel = make(chan struct{})
	r.lastRunnerOpName = opName
	r.lastRunnerOpId = opId
	r.runnerLock.Unlock()
	return nil
}

func (r *UnitLifecycleRunner) run() {
	// Protect against re-entry
	swapped := r.state.CompareAndSwap(STInitializing, STPaused)
	if !swapped {
		return
	}
	r.notifyCurrentOpComplete()

LifecycleLoop:
	for msg := range r.lifecycleMsgChannel {
		switch msg.msgType {
		case lcmStart:
			lastResult := r.owner.UnitStart()
			r.runnerLock.Lock()
			r.lastRunnerResult = lastResult
			if lastResult.OK {
				r.state.Store(STStarted)
			} else {
				r.state.Store(STPaused)
			}
			r.runnerLock.Unlock()
			r.notifyCurrentOpComplete()
		case lcmPause:
			lastResult := r.owner.UnitPause()
			r.runnerLock.Lock()
			r.lastRunnerResult = lastResult
			if lastResult.OK {
				r.state.Store(STPaused)
			} else {
				r.state.Store(STStarted)
			}
			r.runnerLock.Unlock()
			r.notifyCurrentOpComplete()
		case lcmQuit:
			lastResult := r.owner.UnitQuit()
			r.runnerLock.Lock()
			r.lastRunnerResult = lastResult
			if lastResult.OK {
				r.state.Store(STQuit)
				r.runnerLock.Unlock()
				r.notifyCurrentOpComplete()
				break LifecycleLoop
			} else {
				r.state.Store(STPaused)
				r.runnerLock.Unlock()
				r.notifyCurrentOpComplete()
			}

		default:
			panic(fmt.Sprintf("igulib/app.UnitLifecycleRunner.Run(): received message of unknown type %q", msg.msgType))
		}
	}

}

// Name returns Unit's name.
func (r *UnitLifecycleRunner) Name() string {
	return r.name
}

func (r *UnitLifecycleRunner) notifyCurrentOpComplete() {
	swapped := r.runnerOpDoneChannelOpen.CompareAndSwap(true, false)
	if swapped {
		r.runnerLock.Lock()
		close(r.runnerOpDoneChannel)
		r.runnerLock.Unlock()
	}
}

// UnitLifecycleRunner states
const (
	STNotInitialized int32 = iota
	STInitializing
	STPaused
	STStarting
	STStarted
	STPausing
	STQuitting
	STQuit
)

type lifecycleMsg struct {
	msgType string
}

var (
	lcmStart = "start"
	lcmPause = "pause"
	lcmQuit  = "quit"
)

const (
	umcoIdle int32 = iota
	umcoInit
	umcoStart
	umcoPause
	umcoQuit
	umcoGetInfo
	umcoModifyScheme
)

// UnitManagerCurrentOperation string IDs
var (
	UmcoIdle         = "idle"
	UmcoInit         = "init"
	UmcoStart        = "start"
	UmcoPause        = "pause"
	UmcoQuit         = "quit"
	UmcoGetInfo      = "get_info"
	UmcoModifyScheme = "modify_scheme"
)

var umcoStrings = []string{
	UmcoIdle, UmcoInit, UmcoStart, UmcoPause, UmcoQuit, UmcoGetInfo, UmcoModifyScheme,
}

// NegativeOpId identifies an operation that doesn't exist
// or haven't started due to error.
const NegativeOpId int64 = -1
