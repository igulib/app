package app_test

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"testing"

	_ "unsafe" // required for accessing a private variable from app package

	"github.com/igulib/app"
	"github.com/igulib/app/test/test_unit"
	"github.com/igulib/app/test/test_unit/test_unit_impl"

	"github.com/stretchr/testify/require"
)

func TestUnitManagerSchemeOperations(t *testing.T) {
	test_unit_impl.ResetTrackers()

	m := app.NewUnitManager()

	n1 := "tu1"
	n2 := "tu2"
	n3 := "tu3"
	n4 := "tu4"

	// Add 4 units
	tu1 := test_unit_impl.NewTestUnit(n1)
	err := m.AddUnit(tu1)
	require.Equal(t, nil, err)

	tu2 := test_unit_impl.NewTestUnit(n2)
	err = m.AddUnit(tu2)
	require.Equal(t, nil, err)

	tu3 := test_unit_impl.NewTestUnit(n3)
	err = m.AddUnit(tu3)
	require.Equal(t, nil, err)

	tu4 := test_unit_impl.NewTestUnit(n4)
	err = m.AddUnit(tu4)
	require.Equal(t, nil, err)

	// Get reference to the first unit by its name and cast it to
	// its facade interface.
	unit, err := m.GetUnit(n1)
	require.Equal(t, nil, err)
	tuif1, ok := unit.(test_unit.TestUnit)
	require.Equal(t, true, ok, "app.IUnit must successfully cast to test_unit.TestUnit")

	operationScheme := []app.MultiUnitOperationConfig{
		{
			UnitNames:          []string{n1},
			StartTimeoutMillis: 100,
			PauseTimeoutMillis: 100,
			QuitTimeoutMillis:  100,
		},
		{
			UnitNames:          []string{n2, n3},
			StartTimeoutMillis: 100,
			PauseTimeoutMillis: 100,
			QuitTimeoutMillis:  100,
		},
		{
			UnitNames:          []string{n4},
			StartTimeoutMillis: 100,
			PauseTimeoutMillis: 100,
			QuitTimeoutMillis:  100,
		},
	}
	err = m.SetMainOperationScheme(operationScheme)
	require.Equal(t, nil, err)

	// Repeat start and pause several times
	for i := 0; i < 2; i++ {
		// Start single unit
		opId, err := m.Start(n2)
		require.Equal(t, nil, err)
		r := m.WaitForCompletion()
		require.Equal(t, true, r.OK, "single unit must successfully start")
		require.Equal(t, opId, r.OpId, "operation IDs must match")

		// Start all units of the scheme
		opId, _, err = m.StartScheme()
		require.Equal(t, nil, err)

		_, _, err = m.StartScheme()
		require.Equal(t, app.ErrBusy, err,
			"must return error when previous operation is not complete")

		r = m.WaitForCompletion()
		if !r.OK || r.CollateralErrors {
			fmt.Printf("result: %+v\n", r)
		}
		require.True(t, r.OK, "scheme must start successfully")
		require.False(t, r.CollateralErrors, "no collateral errors expected")
		require.Equal(t, r.OpId, opId, "operation IDs must match")

		// Repeated start stress-test
		for x := 0; x < 5; x++ {
			opId, err = m.Start(n1, 100)
			require.Equal(t, nil, err,
				"repeated start must succeed if unit has already started")
			r = m.WaitForCompletion()
			require.Equal(t, r.OpId, opId, "operation IDs must match")
		}

		// Repeated scheme start stress-test
		for x := 0; x < 5; x++ {
			opId, _, err = m.StartScheme()
			require.Equal(t, nil, err,
				"repeated scheme start must succeed if unit has already started")
			r = m.WaitForCompletion()
			require.Equal(t, r.OpId, opId, "operation IDs must match")
		}

		// Unit's API must be available
		av := tuif1.UnitAvailability()
		require.Equal(t, app.UAvailable, av,
			"unit 1 API must be available after successful start")
		av = tu2.UnitAvailability()
		require.Equal(t, app.UAvailable, av,
			"unit 2 API must be available after successful start")
		av = tu3.UnitAvailability()
		require.Equal(t, app.UAvailable, av,
			"unit 3 API must be available after successful start")
		av = tu4.UnitAvailability()
		require.Equal(t, app.UAvailable, av,
			"unit 4 API must be available after successful start")
		respChan, err := tuif1.SendApiRequest("test_msg", 0)
		require.Equal(t, nil, err)
		response, err := tuif1.WaitForApiResponse(respChan, 100)
		require.Equal(t, nil, err)
		require.Equal(t, "test_msg", response, "api response must echo message back")

		// Pause single unit
		opId, err = m.Pause(n3)
		require.Equal(t, nil, err)
		r = m.WaitForCompletion()
		require.Equal(t, true, r.OK, "single unit must be successfully paused")
		require.Equal(t, opId, r.OpId, "operation IDs must match")

		// Pause rest of the units of the scheme
		opId, _, err = m.PauseScheme()
		require.Equal(t, nil, err)
		r = m.WaitForCompletion()
		if !r.OK || r.CollateralErrors {
			fmt.Printf("result: %+v\n", r)
		}
		require.True(t, r.OK, "scheme must pause successfully")
		require.False(t, r.CollateralErrors, "no collateral errors expected")
		require.NotEqual(t, int32(0), tuif1.GoroutineCount(),
			"internal goroutine must not exit on pause")
		require.Equal(t, r.OpId, opId, "operation IDs must match")

		// Unit's API must NOT be available
		av = tuif1.UnitAvailability()
		require.Contains(t, []app.UnitAvailability{
			app.UNotAvailable, app.UTemporarilyUnavailable}, av,
			"unit's API must NOT be available when unit paused")
		_, err = tuif1.SendApiRequest("test_msg", 0)
		require.Equal(t, test_unit.ErrNotAvailable, err)

		// Repeated pause stress-test
		for x := 0; x < 5; x++ {
			opId, err = m.Pause(n1, 100)
			require.Equal(t, nil, err,
				"repeated pause must succeed if unit has already paused")
			r = m.WaitForCompletion()
			require.Equal(t, r.OpId, opId, "operation IDs must match")
		}

		// Repeated scheme pause stress-test
		for x := 0; x < 5; x++ {
			opId, _, err = m.PauseScheme()
			require.Equal(t, nil, err,
				"repeated scheme pause must succeed if unit has already paused")
			r = m.WaitForCompletion()
			require.Equal(t, r.OpId, opId, "operation IDs must match")
		}
	}

	// Quit single unit
	opId, err := m.Quit(n2)
	require.Equal(t, nil, err)
	r := m.WaitForCompletion()
	require.Equal(t, true, r.OK, "single unit must successfully quit")
	require.Equal(t, opId, r.OpId, "operation IDs must match")

	// Quit all units
	opId, _, err = m.QuitAll()
	require.Equal(t, nil, err)
	result := m.WaitForCompletion()
	if !result.OK || result.CollateralErrors {
		fmt.Printf("result: %+v\n", result)
	}
	require.True(t, result.OK, "all units must quit successfully")
	require.False(t, result.CollateralErrors, "no collateral errors expected")
	require.EqualValues(t, 0, tuif1.GoroutineCount(),
		"there must be no goroutine leakage after unit quits")
	require.Equal(t, result.OpId, opId, "operation IDs must match")

	// Unit's API must NOT be available after unit quit
	av := tuif1.UnitAvailability()
	require.Contains(t, []app.UnitAvailability{
		app.UNotAvailable}, av,
		"unit's API must NOT be available after quit")
	_, err = tuif1.SendApiRequest("test_msg", 0)
	require.Equal(t, test_unit.ErrNotAvailable, err)

	// Repeated quit stress-test
	for x := 0; x < 5; x++ {
		opId, err = m.Quit(n1, 100)
		require.Equal(t, nil, err, "repeated Quit must succeed if unit has already quit")
		result = m.WaitForCompletion()
		require.Equal(t, result.OpId, opId, "operation IDs must match")
	}

	// Repeated quit all stress-test
	for x := 0; x < 5; x++ {
		opId, _, err = m.QuitAll()
		require.Equal(t, nil, err, "repeated QuitAll must succeed if units have already quit")
		result = m.WaitForCompletion()
		require.Equal(t, result.OpId, opId, "operation IDs must match")
	}

	require.EqualValues(t, 0, tuif1.GoroutineCount(),
		"there must be no goroutine leakage after unit quits")
}

func TestTimeouts(t *testing.T) {
	test_unit_impl.ResetTrackers()

	m := app.NewUnitManager()

	n1 := "tu1"
	n2 := "tu2"
	n3 := "tu3"
	n4 := "tu4"

	// Protect the integrity of a series of UnitManager operations.
	// It has no use here except demo because this test uses a single goroutine
	// to call UnitManager methods.
	m.Lock.Lock()
	defer m.Lock.Unlock()

	// Add 4 units
	tu1 := test_unit_impl.NewTestUnit(n1)
	err := m.AddUnit(tu1)
	require.Equal(t, nil, err)

	tu2 := test_unit_impl.NewTestUnit(n2)
	err = m.AddUnit(tu2)
	require.Equal(t, nil, err)

	tu3 := test_unit_impl.NewTestUnit(n3)
	err = m.AddUnit(tu3)
	require.Equal(t, nil, err)

	tu4 := test_unit_impl.NewTestUnit(n4)
	err = m.AddUnit(tu4)
	require.Equal(t, nil, err)

	operationScheme := []app.MultiUnitOperationConfig{
		{
			UnitNames:          []string{n1},
			StartTimeoutMillis: 100,
			PauseTimeoutMillis: 100,
			QuitTimeoutMillis:  100,
		},
		{
			UnitNames:          []string{n2, n3},
			StartTimeoutMillis: 100,
			PauseTimeoutMillis: 100,
			QuitTimeoutMillis:  100,
		},
		{
			UnitNames:          []string{n4},
			StartTimeoutMillis: 100,
			PauseTimeoutMillis: 100,
			QuitTimeoutMillis:  100,
		},
	}
	err = m.SetMainOperationScheme(operationScheme)
	require.Equal(t, nil, err)

	// Make tu1 produce a timeout on start
	simulateTimeout := test_unit.OpParameters{
		PeriodMillis: 150,
	}
	tu1.SetStartParameters(simulateTimeout)

	opId, err := m.Start(n1)
	require.Equal(t, nil, err)
	r := m.WaitForCompletion()

	require.Equal(t, r.OpId, opId, "opIds must be equal")
	require.Equal(t, false, r.OK)
	require.Equal(t, app.ErrTimedOut, r.ResultMap[n1].CollateralError,
		"timeout must occur at first start try")

	state := tu1.Runner().State()
	require.Equal(t, app.STStarting, state, "tu1 must be in STStarting after first start try")

	opId, err = m.Start(n1)
	require.Equal(t, nil, err)

	r = m.WaitForCompletion()
	state = tu1.Runner().State()

	require.Equal(t, app.STStarted, state, "tu1 must be in STStarted after second start try")
	require.Equal(t, r.OpId, opId, "opIds must be equal")
	require.Equal(t, true, r.OK, "tu1 must successfully start at second try")

	// Start scheme when one of the units has already started
	opId, _, err = m.StartScheme()
	require.Equal(t, nil, err)
	r = m.WaitForCompletion()
	require.Equal(t, r.OpId, opId, "opIds must be equal")
	require.Equal(t, true, r.OK,
		"scheme must successfully start when one of the units has already started")
	require.Equal(t, false, r.CollateralErrors)

	// Pause scheme without timeout
	opId, _, err = m.PauseScheme()
	require.Equal(t, nil, err)
	r = m.WaitForCompletion()
	require.Equal(t, r.OpId, opId, "opIds must be equal")
	require.Equal(t, true, r.OK,
		"PauseScheme is must be successful")
	require.Equal(t, false, r.CollateralErrors)

	tu1.SetPauseParameters(simulateTimeout)

	// Test schemes

	// Start scheme first time when tu1 simulates timeout
	opId, _, err = m.StartScheme()
	require.Equal(t, nil, err)
	r = m.WaitForCompletion()
	require.Equal(t, r.OpId, opId, "opIds must be equal")
	require.Equal(t, false, r.OK,
		"tu1 expected to fail at StartScheme first call when tu1 simulates timeout")
	require.Equal(t, app.ErrTimedOut, r.ResultMap[n1].CollateralError,
		"timeout must occur at StartScheme first call when tu1 simulates timeout")

	require.Equal(t, app.ErrSkippedByUnitManager, r.ResultMap[n2].CollateralError,
		"subsequent layer start must be skipped as current layer operation failed.")
	require.Equal(t, app.ErrSkippedByUnitManager, r.ResultMap[n3].CollateralError,
		"subsequent layer start must be skipped as current layer operation failed.")
	require.Equal(t, app.ErrSkippedByUnitManager, r.ResultMap[n4].CollateralError,
		"subsequent layer start must be skipped as current layer operation failed.")

	// Start scheme second time when tu1 simulates timeout
	opId, _, err = m.StartScheme()
	require.Equal(t, nil, err)
	r = m.WaitForCompletion()
	require.Equal(t, r.OpId, opId, "opIds must be equal")

	state = tu1.Runner().State()
	require.Equal(t, app.STStarted, state,
		"tu1 must be in STStarted after second scheme start try")
	require.Equal(t, true, r.OK,
		"tu1 must succeed at StartScheme second call when tu1 simulates timeout")
	require.Equal(t, false, r.CollateralErrors)

	// Pause scheme when tu1 simulates timeout
	tu1.SetPauseParameters(simulateTimeout)

	opId, _, err = m.PauseScheme()
	require.Equal(t, nil, err)
	r = m.WaitForCompletion()

	require.Equal(t, r.OpId, opId, "opIds must be equal")
	require.Equal(t, false, r.OK,
		"tu1 expected to fail with timeout when scheme paused")
	require.Equal(t, app.ErrTimedOut, r.ResultMap[n1].CollateralError,
		"timeout must occur at PauseScheme first call")
	require.Equal(t, true, r.ResultMap[n2].OK && r.ResultMap[n3].OK && r.ResultMap[n4].OK,
		"other units must pause successfully even if one unit fails")

	// Pause scheme second attempt
	opId, _, err = m.PauseScheme()
	require.Equal(t, nil, err)
	r = m.WaitForCompletion()

	require.Equal(t, r.OpId, opId, "opIds must be equal")
	require.Equal(t, true, r.OK,
		"tu1 expected to succeed on second PauseScheme attempt")

	// QuitAll when tu1 simulates timeout
	tu1.SetQuitParameters(simulateTimeout)

	opId, _, err = m.QuitAll()
	require.Equal(t, nil, err)
	r = m.WaitForCompletion()

	require.Equal(t, r.OpId, opId, "opIds must be equal")
	require.Equal(t, false, r.OK,
		"tu1 expected to fail with timeout while QuitAll")
	require.Equal(t, app.ErrTimedOut, r.ResultMap[n1].CollateralError,
		"timeout must occur at QuitAll first call")
	require.Equal(t, true, r.ResultMap[n2].OK && r.ResultMap[n3].OK && r.ResultMap[n4].OK,
		"other units must quit successfully even if one unit fails")

	// QuitAll second attempt
	opId, _, err = m.QuitAll()
	require.Equal(t, nil, err)
	r = m.WaitForCompletion()

	require.Equal(t, r.OpId, opId, "opIds must be equal")
	require.Equal(t, true, r.OK,
		"tu1 expected to succeed on second QuitAll attempt")
}

func TestListUnitStates(t *testing.T) {
	test_unit_impl.ResetTrackers()

	m := app.NewUnitManager()

	n1 := "tu1"
	n2 := "tu2"
	n3 := "tu3"
	n4 := "tu4"

	// Add 4 units
	tu1 := test_unit_impl.NewTestUnit(n1)
	err := m.AddUnit(tu1)
	require.Equal(t, nil, err)

	tu2 := test_unit_impl.NewTestUnit(n2)
	err = m.AddUnit(tu2)
	require.Equal(t, nil, err)

	tu3 := test_unit_impl.NewTestUnit(n3)
	err = m.AddUnit(tu3)
	require.Equal(t, nil, err)

	tu4 := test_unit_impl.NewTestUnit(n4)
	err = m.AddUnit(tu4)
	require.Equal(t, nil, err)

	operationScheme := []app.MultiUnitOperationConfig{
		{
			UnitNames: []string{n1},
		},
		{
			UnitNames: []string{n2, n3},
		},
		{
			UnitNames: []string{n4},
		},
	}
	err = m.SetMainOperationScheme(operationScheme)
	require.Equal(t, nil, err)

	states, err := m.ListUnitStates()
	require.Equal(t, nil, err)
	require.Equal(t, map[string]int32{
		n1: app.STPaused,
		n2: app.STPaused,
		n3: app.STPaused,
		n4: app.STPaused,
	}, states)

	_, _, _ = m.StartScheme()
	_ = m.WaitForCompletion()

	states, err = m.ListUnitStates()
	require.Equal(t, nil, err)
	require.Equal(t, map[string]int32{
		n1: app.STStarted,
		n2: app.STStarted,
		n3: app.STStarted,
		n4: app.STStarted,
	}, states)

	_, _, _ = m.PauseScheme()
	_ = m.WaitForCompletion()

	states, err = m.ListUnitStates()
	require.Equal(t, nil, err)
	require.Equal(t, map[string]int32{
		n1: app.STPaused,
		n2: app.STPaused,
		n3: app.STPaused,
		n4: app.STPaused,
	}, states)

	_, _ = m.Quit(n1)
	_ = m.WaitForCompletion()

	states, err = m.ListUnitStates()
	require.Equal(t, nil, err)
	require.Equal(t, map[string]int32{
		n1: app.STQuit,
		n2: app.STPaused,
		n3: app.STPaused,
		n4: app.STPaused,
	}, states)

	_, _, _ = m.QuitAll()
	_, err = m.GetUnit(n1)
	_, err2 := m.ListUnitStates()
	require.Equal(t, app.ErrBusy, err)
	require.Equal(t, app.ErrBusy, err2)
	_ = m.WaitForCompletion()

	states, err = m.ListUnitStates()
	require.Equal(t, nil, err)
	require.Equal(t, map[string]int32{
		n1: app.STQuit,
		n2: app.STQuit,
		n3: app.STQuit,
		n4: app.STQuit,
	}, states)

	require.Equal(t, n1, tu1.Runner().Name())
	_, err = m.GetUnit("not_exists")
	require.Equal(t, app.ErrUnitNotFound, err)
}

func TestErrors(t *testing.T) {
	test_unit_impl.ResetTrackers()

	m := app.NewUnitManager()

	n1 := "tu1"
	n2 := "tu2"
	n3 := "tu3"
	n4 := "tu4"

	// Add 4 units
	tu1 := test_unit_impl.NewTestUnit(n1)
	err := m.AddUnit(tu1)
	require.Equal(t, nil, err)

	err = m.AddUnit(tu1)
	require.Equal(t, app.ErrUnitAlreadyExists, err,
		"must return error if unit with specified name already exists")

	tu2 := test_unit_impl.NewTestUnit(n2)
	err = m.AddUnit(tu2)
	require.Equal(t, nil, err)

	tu3 := test_unit_impl.NewTestUnit(n3)
	err = m.AddUnit(tu3)
	require.Equal(t, nil, err)

	tu4 := test_unit_impl.NewTestUnit(n4)
	err = m.AddUnit(tu4)
	require.Equal(t, nil, err)

	operationScheme := []app.MultiUnitOperationConfig{
		{
			UnitNames:          []string{n1},
			StartTimeoutMillis: 100,
			PauseTimeoutMillis: 100,
			QuitTimeoutMillis:  100,
		},
		{
			UnitNames:          []string{n2, n3},
			StartTimeoutMillis: 100,
			PauseTimeoutMillis: 100,
			QuitTimeoutMillis:  100,
		},
		{
			UnitNames:          []string{n4},
			StartTimeoutMillis: 100,
			PauseTimeoutMillis: 100,
			QuitTimeoutMillis:  100,
		},
	}
	err = m.SetMainOperationScheme(operationScheme)
	require.Equal(t, nil, err)

	// Make tu1 produce an error on start
	produceError := test_unit.OpParameters{
		PeriodMillis:            30,
		SimulateFailure:         true,
		SimulateCollateralError: true,
	}
	tu2.SetStartParameters(produceError)

	opId, err := m.Start(n2)
	require.Equal(t, nil, err)
	r := m.WaitForCompletion()

	require.Equal(t, r.OpId, opId, "opIds must be equal")
	require.Equal(t, false, r.OK)
	require.Equal(t, test_unit.ErrDummy, r.ResultMap[n2].CollateralError,
		"dummy error is expected to occur")

	state := tu2.Runner().State()
	require.Equal(t, app.STPaused, state, "unit state must not change when start failed")

	// Test StartScheme error
	opId, _, err = m.StartScheme()
	require.Equal(t, nil, err)
	r = m.WaitForCompletion()
	require.Equal(t, r.OpId, opId, "opIds must be equal")
	require.Equal(t, false, r.OK)
	require.Equal(t, true, r.CollateralErrors)
	require.Equal(t, test_unit.ErrDummy, r.ResultMap[n2].CollateralError,
		"dummy error is expected to occur")

	require.Equal(t, app.STStarted, tu1.Runner().State(),
		"unit state must not change when start failed")
	require.Equal(t, app.STPaused, tu2.Runner().State(),
		"unit state must not change when start failed")
	require.Equal(t, app.STStarted, tu3.Runner().State(),
		"other units in the same layer must start successfully")
	require.Equal(t, app.STPaused, tu4.Runner().State(),
		"units in subsequent layers must not start")

	// Start scheme without errors
	tu2.ResetDefaults()

	_, _, _ = m.StartScheme()
	r = m.WaitForCompletion()
	require.Equal(t, true, r.OK)

	// Test PauseScheme error
	test_unit_impl.ResetTrackers()
	tu2.SetPauseParameters(produceError)

	opId, _, err = m.PauseScheme()
	require.Equal(t, nil, err)
	r = m.WaitForCompletion()

	require.Equal(t, r.OpId, opId, "opIds must be equal")
	require.Equal(t, false, r.OK)
	require.Equal(t, true, r.CollateralErrors)
	require.Equal(t, test_unit.ErrDummy, r.ResultMap[n2].CollateralError,
		"dummy error is expected to occur")

	require.Equal(t, app.STPaused, tu1.Runner().State(),
		"tu1 must be paused last")
	require.Equal(t, app.STStarted, tu2.Runner().State(),
		"tu2 must fail to pause")
	require.Equal(t, app.STPaused, tu3.Runner().State(),
		"tu3 must pause")
	require.Equal(t, app.STPaused, tu4.Runner().State(),
		"tu4 must pause")

	require.Equal(t, []string{"tu4", "tu3", "tu1"}, test_unit_impl.PauseOrderTracker.Get(),
		"pause order must be reverse of start order")

	// Test QuitAll error
	test_unit_impl.ResetTrackers()
	tu2.ResetDefaults()

	opId, badStateUnits, err := m.QuitAll()
	require.Equal(t, app.ErrUnitHasUnexpectedState, err)
	require.Equal(t, []string{n2}, badStateUnits,
		"tu2 can't quit because it is in STStarted state")
	r = m.WaitForCompletion()

	require.Equal(t, r.OpId, opId, "opIds must be equal")
	require.Equal(t, false, r.OK)
	require.Equal(t, true, r.CollateralErrors,
		"tu2 has bad initial state")
	require.Equal(t, app.ErrUnitHasUnexpectedState, r.ResultMap[n2].CollateralError,
		"dummy error is expected to occur")

	opId, err = m.Quit(n2)
	require.Equal(t, app.ErrUnitHasUnexpectedState, err)
	require.Equal(t, app.NegativeOpId, opId)

	require.Equal(t, app.STQuit, tu1.Runner().State())
	require.Equal(t, app.STStarted, tu2.Runner().State())
	require.Equal(t, app.STQuit, tu3.Runner().State())
	require.Equal(t, app.STQuit, tu4.Runner().State())

	_, _ = m.Pause(n2)
	r = m.WaitForCompletion()
	require.Equal(t, true, r.OK)

	_, _, _ = m.QuitAll()
	r = m.WaitForCompletion()
	require.Equal(t, true, r.OK, "no errors because rest of the units are already in STQuit")
	require.Equal(t, app.STQuit, tu2.Runner().State())

	require.EqualValues(t, 0, tu1.GoroutineCount(),
		"must be no goroutine leakage after unit quits")
	require.EqualValues(t, 0, tu2.GoroutineCount(),
		"must be no goroutine leakage after unit quits")
	require.EqualValues(t, 0, tu3.GoroutineCount(),
		"must be no goroutine leakage after unit quits")
	require.EqualValues(t, 0, tu4.GoroutineCount(),
		"must be no goroutine leakage after unit quits")
}

func TestSetCustomShutdownScheme(t *testing.T) {
	test_unit_impl.ResetTrackers()

	m := app.NewUnitManager()

	n1 := "tu1"
	n2 := "tu2"
	n3 := "tu3"

	// Add 3 units
	tu1 := test_unit_impl.NewTestUnit(n1)
	err := m.AddUnit(tu1)
	require.Equal(t, nil, err)

	tu2 := test_unit_impl.NewTestUnit(n2)
	err = m.AddUnit(tu2)
	require.Equal(t, nil, err)

	tu3 := test_unit_impl.NewTestUnit(n3)
	err = m.AddUnit(tu3)
	require.Equal(t, nil, err)

	operationScheme := []app.MultiUnitOperationConfig{
		{
			UnitNames:          []string{n1},
			StartTimeoutMillis: 100,
			PauseTimeoutMillis: 100,
			QuitTimeoutMillis:  100,
		},
		{
			UnitNames:          []string{n3},
			StartTimeoutMillis: 100,
			PauseTimeoutMillis: 100,
			QuitTimeoutMillis:  100,
		},
		{
			UnitNames:          []string{n2},
			StartTimeoutMillis: 100,
			PauseTimeoutMillis: 100,
			QuitTimeoutMillis:  100,
		},
	}
	err = m.SetMainOperationScheme(operationScheme)
	require.Equal(t, nil, err)

	customShutdownScheme := []app.MultiUnitOperationConfig{
		{
			UnitNames:          []string{n3},
			StartTimeoutMillis: 100,
			PauseTimeoutMillis: 100,
			QuitTimeoutMillis:  200,
		},
		{
			UnitNames:          []string{n1},
			StartTimeoutMillis: 100,
			PauseTimeoutMillis: 100,
			QuitTimeoutMillis:  200,
		},
		{
			UnitNames:          []string{n2},
			StartTimeoutMillis: 100,
			PauseTimeoutMillis: 100,
			QuitTimeoutMillis:  200,
		},
	}

	err = m.SetCustomShutdownScheme(customShutdownScheme)
	require.Equal(t, nil, err)

	simulateTimeout := test_unit.OpParameters{
		PeriodMillis: 150,
	}
	tu2.SetQuitParameters(simulateTimeout)

	simulateFailure := test_unit.OpParameters{
		PeriodMillis:    30,
		SimulateFailure: true,
	}
	tu3.SetStartParameters(simulateFailure)

	// StartScheme with failure simulated
	opId, _, err := m.StartScheme()
	require.Equal(t, nil, err)
	r := m.WaitForCompletion()
	require.Equal(t, r.OpId, opId, "opIds must be equal")
	require.Equal(t, false, r.OK)
	require.Equal(t, true, r.CollateralErrors, "collateral error: tu2 start skipped")

	require.Equal(t, []string{"tu1"}, test_unit_impl.StartOrderTracker.Get())

	require.Equal(t, app.STStarted, tu1.Runner().State(),
		"unit must start successfully")
	require.Equal(t, app.STPaused, tu3.Runner().State(),
		"unit start must fail")
	require.Equal(t, app.STPaused, tu2.Runner().State(),
		"unit start must be skipped")

	tu3.ResetDefaults()
	test_unit_impl.StartOrderTracker.Reset()

	// StartScheme successfully
	_, _, _ = m.StartScheme()
	r = m.WaitForCompletion()
	require.Equal(t, true, r.OK)
	require.Equal(t, false, r.CollateralErrors)
	require.Equal(t, []string{"tu3", "tu2"}, test_unit_impl.StartOrderTracker.Get(),
		"unit tu1 already started so it is not in the list")

	_, _, _ = m.PauseScheme()
	r = m.WaitForCompletion()
	require.Equal(t, true, r.OK)
	require.Equal(t, false, r.CollateralErrors)
	require.Equal(t, []string{"tu3", "tu1", "tu2"}, test_unit_impl.PauseOrderTracker.Get(),
		"custom pause scheme must be used")

	_, _, _ = m.QuitAll()
	r = m.WaitForCompletion()
	require.Equal(t, true, r.OK)
	require.Equal(t, false, r.CollateralErrors)

	require.EqualValues(t, 0, tu1.GoroutineCount(),
		"must be no goroutine leakage after unit quits")
	require.EqualValues(t, 0, tu2.GoroutineCount(),
		"must be no goroutine leakage after unit quits")
	require.EqualValues(t, 0, tu3.GoroutineCount(),
		"must be no goroutine leakage after unit quits")
}

func TestOperationAtomicity(t *testing.T) {
	test_unit_impl.ResetTrackers()

	m := app.NewUnitManager()

	n1 := "tu1"

	// Add 3 units
	tu1 := test_unit_impl.NewTestUnit(n1)
	err := m.AddUnit(tu1)
	require.Equal(t, nil, err)

	opScheme := []app.MultiUnitOperationConfig{
		{
			UnitNames:          []string{n1},
			StartTimeoutMillis: 100,
			PauseTimeoutMillis: 100,
			QuitTimeoutMillis:  100,
		},
	}
	err = m.SetMainOperationScheme(opScheme)
	require.Equal(t, nil, err)

	_, _, err = m.StartScheme()
	require.Equal(t, nil, err)

	// At this point UnitManager must be busy
	// and methods must return ErrBusy.
	err = m.AddUnit(tu1)
	require.Equal(t, app.ErrBusy, err)
	_, err = m.GetUnit(n1)
	require.Equal(t, app.ErrBusy, err)
	_, err = m.ListUnitStates()
	require.Equal(t, app.ErrBusy, err)

	err = m.SetMainOperationScheme(opScheme)
	require.Equal(t, app.ErrBusy, err)
	err = m.SetCustomShutdownScheme(opScheme)
	require.Equal(t, app.ErrBusy, err)

	_, err = m.Start(n1)
	require.Equal(t, app.ErrBusy, err)
	_, err = m.Pause(n1)
	require.Equal(t, app.ErrBusy, err)
	_, err = m.Quit(n1)
	require.Equal(t, app.ErrBusy, err)

	_, _, err = m.StartScheme()
	require.Equal(t, app.ErrBusy, err)
	_, _, err = m.PauseScheme()
	require.Equal(t, app.ErrBusy, err)
	_, _, err = m.QuitAll()
	require.Equal(t, app.ErrBusy, err)

	_ = m.WaitForCompletion()
	_, _, _ = m.PauseScheme()
	_ = m.WaitForCompletion()
	_, _, _ = m.QuitAll()
	r := m.WaitForCompletion()
	require.Equal(t, true, r.OK)
}

func TestMultipleConcurrentApiRequests(t *testing.T) {
	test_unit_impl.ResetTrackers()
	n1 := "tu1"
	m := app.NewUnitManager()
	tu1 := test_unit_impl.NewTestUnit(n1)
	err := m.AddUnit(tu1)
	require.Equal(t, nil, err)
	_, _ = m.Start(n1)
	r := m.WaitForCompletion()
	require.Equal(t, true, r.OK)

	require.Equal(t, app.UAvailable, tu1.UnitAvailability())

	const total = 100
	const TestMsg = "dummy_message"
	wg := sync.WaitGroup{}
	wg.Add(total)
	for x := 0; x < total; x++ {
		go func() {
			defer wg.Done()
			respChan, err := tu1.SendApiRequest(TestMsg, 100)
			require.Equal(t, nil, err)

			msg, err := tu1.WaitForApiResponse(respChan, 200)
			require.Equal(t, nil, err)
			require.Equal(t, TestMsg, msg)
		}()
	}
	wg.Wait()
	_, _ = m.Pause(n1)
	r = m.WaitForCompletion()
	require.Equal(t, true, r.OK)

	_, _ = m.Quit(n1)
	r = m.WaitForCompletion()
	require.Equal(t, true, r.OK)

	require.EqualValues(t, 0, tu1.GoroutineCount(), "goroutines must not leak")
}

func TestWaitConcurrently(t *testing.T) {
	test_unit_impl.ResetTrackers()
	n1 := "tu1"
	m := app.NewUnitManager()
	tu1 := test_unit_impl.NewTestUnit(n1)
	err := m.AddUnit(tu1)
	require.Equal(t, nil, err)
	opId, _ := m.Start(n1)

	const total = 10
	wg := sync.WaitGroup{}
	wg.Add(total)
	for x := 0; x < total; x++ {
		go func() {
			defer wg.Done()
			r := m.WaitForCompletion()
			require.Equal(t, true, r.OK)
			require.Equal(t, false, r.CollateralErrors)
			require.Equal(t, opId, r.OpId)
			require.Equal(t, app.UAvailable, tu1.UnitAvailability())
		}()
	}
	wg.Wait()
	_, _ = m.Pause(n1)
	r := m.WaitForCompletion()
	require.Equal(t, true, r.OK)

	_, _ = m.Quit(n1)
	r = m.WaitForCompletion()
	require.Equal(t, true, r.OK)

	require.EqualValues(t, 0, tu1.GoroutineCount(), "goroutines must not leak")
}

//go:linkname sysSignalChan github.com/igulib/app.sysSignalChan
var sysSignalChan chan os.Signal

func TestSignals(t *testing.T) {

	err := app.EnableSignalInterception(true, true)
	require.Equal(t, nil, err)

	err = app.EnableSignalInterception(true, true)
	require.Equal(t, app.ErrAlreadyEnabled, err)

	err = app.DisableSignalInterception()
	require.Equal(t, nil, err)

	err = app.DisableSignalInterception()
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

	err = app.DisableSignalInterception()
	require.Equal(t, app.ErrShuttingDown, err)

	require.Equal(t, true, app.IsShuttingDown())
}
