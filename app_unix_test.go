package app_test

import (
	"os"
	"sync"
	"syscall"
	"testing"

	_ "unsafe" // required for accessing a private variable from app package

	"github.com/igulib/app"
	"github.com/stretchr/testify/require"
)

//go:linkname sysSignalChan github.com/igulib/app.sysSignalChan
var sysSignalChan chan os.Signal

func TestSignals(t *testing.T) {

	err := app.EnableSignalInterception(true, true)
	require.Equal(t, nil, err)

	err = app.EnableSignalInterception(true, true)
	require.Equal(t, app.ErrAlreadyEnabled, err)

	err = app.DisableSysSignalInterception()
	require.Equal(t, nil, err)

	err = app.DisableSysSignalInterception()
	require.Equal(t, app.ErrAlreadyDisabled, err)

	err = app.EnableSignalInterception(true, true)
	require.Equal(t, nil, err)

	const total = 10
	wg := sync.WaitGroup{}
	wg.Add(total)
	for x := 0; x < total; x++ {
		go func() {
			defer wg.Done()
			app.WaitUntilGlobalShutdownInitiated()
			require.Equal(t, true, app.IsShuttingDown())
		}()
	}

	sysSignalChan <- syscall.SIGINT // emulate SIGINT
	wg.Wait()

	err = app.EnableSignalInterception(true, true)
	require.Equal(t, app.ErrShuttingDown, err)

	err = app.DisableSysSignalInterception()
	require.Equal(t, app.ErrShuttingDown, err)

	require.Equal(t, true, app.IsShuttingDown())
}
