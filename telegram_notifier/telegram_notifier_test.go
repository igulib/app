package telegram_notifier

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/igulib/app"
	"github.com/stretchr/testify/require"
)

// TEST SETUP BEGIN

// Temporary directory for all telegram_notifier tests.
var testRootDir string

// Whether to remove testRootDir after all tests done.
var removeTestRootDir = false

// Alias for brevity
var join = filepath.Join

func TestMain(m *testing.M) {
	setup(m)
	code := m.Run()
	teardown(m)
	os.Exit(code)
}

func setup(m *testing.M) {
	fmt.Println("--- telegram_notifier tests setup ---")
	testRootDir = join(os.TempDir(), "telegram_notifier_tests")
	// Remove old testRootDir if exists
	_, err := os.Stat(testRootDir)
	if err == nil {
		err = os.RemoveAll(testRootDir)
		if err != nil {
			panic(fmt.Sprintf("failed to remove existing telegram_notifier test directory '%s': %v\n", testRootDir, err))
		} else {
			fmt.Printf("existing telegram_notifier test dir successfully removed: '%s'\n", testRootDir)
		}
	} else {
		if !os.IsNotExist(err) {
			panic(fmt.Sprintf("os.Stat failed for telegram_notifier test directory '%s': %v\n", testRootDir, err))
		}
	}
	// Create new testDir
	err = os.MkdirAll(testRootDir, 0775)
	if err != nil {
		panic(fmt.Sprintf("failed to create telegram_notifier test directory '%s': %v\n", testRootDir, err))
	}

	fmt.Printf("--- created test dir for telegram_notifier tests: '%s' ---\n", testRootDir)
}

func teardown(m *testing.M) {
	if removeTestRootDir {
		err := os.RemoveAll(testRootDir)
		if err != nil {
			fmt.Fprintf(os.Stderr,
				"failed to remove telegram_notifier test directory: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("--- telegram_notifier test directory successfully removed ---")
	} else {
		fmt.Printf("--- telegram_notifier tests complete. You can remove the test directory manually if required: '%s'\n ---", testRootDir)
	}
}

// TEST SETUP END

func TestDummy(t *testing.T) {
	require.Equal(t, nil, nil)
}

func TestBasicUsage(t *testing.T) {

	configBytes, err := os.ReadFile("./test_data/TestBasicUsage.yaml")
	require.Equal(t, nil, err)

	config, err := ParseYamlConfig([]byte(configBytes))
	require.Equal(t, nil, err)

	unitName := "telegram_notifier"
	tn, err := New(unitName, config)
	require.Equal(t, nil, err, "telegram_notifier must be created successfully")

	err = app.M.AddUnit(tn)
	require.Equal(t, nil, err, "telegram_notifier must be successfully added to UnitManager")

	_, err = app.M.Start(unitName)
	require.Equal(t, nil, err, "telegram_notifier must start successfully")

	err = tn.SendAsync("IGULIB Telegram Notifier Test", fmt.Sprintf("This is a test message sent from telegram_notifier_test.go/TestBasicUsage at %s. OS: %q.\n", time.Now().Format(time.RFC3339), runtime.GOOS))
	require.Equal(t, nil, err, "message must be sent successfully")

	_, err = app.M.Pause(unitName)
	require.Equal(t, nil, err, "telegram_notifier must pause successfully")

	_, err = app.M.Quit(unitName)
	require.Equal(t, nil, err, "telegram_notifier must quit successfully")
}
