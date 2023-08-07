package app_test

import (
	"sync"
	"testing"

	"github.com/igulib/app"
	"github.com/stretchr/testify/require"
)

func TestAppGlobalShutdownMechanism(t *testing.T) {
	require.Equal(t, false, app.IsShuttingDown())

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

	err := app.InitiateShutdown()
	require.Equal(t, nil, err)
	wg.Wait()
	require.Equal(t, true, app.IsShuttingDown())
}
