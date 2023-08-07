package app

import (
	"errors"
	"os"
	"os/signal"
	"sync/atomic"

	"syscall"
)

var (
	sysSignalChan              chan os.Signal
	sysSignalCancelChan        chan struct{}
	sysSignalCancelConfirmChan chan struct{}
	sysSignalOpInProgress      atomic.Bool
	sysSignalsSetUpDone        bool
)

var (
	ErrAlreadyEnabled  = errors.New("already enabled")
	ErrAlreadyDisabled = errors.New("already disabled")
)

// EnableSignalInterception enables your app to intercept
// os signals SIGINT and/or SIGTERM
// to initiate graceful app shutdown
// (SIGKILL can't be intercepted by user application).
// This function is only defined for unix-like systems and is
// not available on Windows.
//
// Returns:
// ErrBusy if previous operation is not complete;
// ErrShuttingDown if app is already shutting down;
// ErrAlreadyEnabled if already enabled.
//
// Example:
//
//	func main() {
//		fmt.Println("== app started ==")
//		// Start app tasks here in separate goroutines...
//		_ = app.EnableSignalInterception(true, true)
//		app.WaitUntilGlobalShutdownInitiated()
//		// Do graceful shutdown routines here...
//		fmt.Println("== app exited ==")
//	}
func EnableSignalInterception(interceptSIGINT, interceptSIGTERM bool) error {
	if !(interceptSIGINT || interceptSIGTERM) {
		return nil
	}
	swapped := sysSignalOpInProgress.CompareAndSwap(false, true)
	if !swapped {
		return ErrBusy
	}
	defer func() {
		sysSignalOpInProgress.CompareAndSwap(true, false)
	}()
	if IsShuttingDown() {
		return ErrShuttingDown
	}

	if sysSignalsSetUpDone {
		return ErrAlreadyEnabled
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
	return nil
}

// DisableSysSignalInterception cancels the effect of
// previously called EnableSysSignalInterception
// and disconnects system signals from global app shutdown mechanism.
// This function is only defined for unix-like systems and is
// not available on Windows.
//
// Returns:
// ErrBusy if previous operation is not complete;
// ErrShuttingDown if app is already shutting down;
// ErrAlreadyDisabled if already canceled or not enabled.
func DisableSysSignalInterception() error {
	swapped := sysSignalOpInProgress.CompareAndSwap(false, true)
	if !swapped {
		return ErrBusy
	}
	defer func() {
		sysSignalOpInProgress.CompareAndSwap(true, false)
	}()

	if IsShuttingDown() {
		return ErrShuttingDown
	}

	if !sysSignalsSetUpDone {
		return ErrAlreadyDisabled
	}

	close(sysSignalCancelChan)
	<-sysSignalCancelConfirmChan
	return nil
}
