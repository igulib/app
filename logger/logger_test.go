package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/igulib/app"
	"github.com/igulib/app/telegram_notifier"
	"github.com/rs/zerolog"

	"github.com/stretchr/testify/require"
)

// testId is used to create a new app/logger unit name for each test.
var testId = 1

// TEST SETUP BEGIN

// Temporary directory for all app/logger tests.
var testRootDir string

// Whether to remove testRootDir after all tests done.
var removeTestRootDir = false

// Alias for brevity
var join = filepath.Join

// Create a sub-directory for a test with the specified name
// inside testRootDir and return path to it.
// Default permissions are 0775.
func createSubDir(name string, perms ...os.FileMode) string {
	var resolvedPerms os.FileMode = 0775
	if len(perms) > 0 {
		resolvedPerms = perms[0]
	}
	newDir := join(testRootDir, name)
	err := os.Mkdir(newDir, resolvedPerms)
	if err != nil {
		panic(
			fmt.Sprintf("failed to create sub-directory: '%v'\n", err))
	}
	return newDir
}

func TestMain(m *testing.M) {
	setup(m)
	code := m.Run()
	teardown(m)
	os.Exit(code)
}

func setup(m *testing.M) {
	fmt.Println("--- app/logger tests setup ---")
	testRootDir = join(os.TempDir(), "app/logger_tests")
	// Remove old testRootDir if exists
	_, err := os.Stat(testRootDir)
	if err == nil {
		err = os.RemoveAll(testRootDir)
		if err != nil {
			panic(fmt.Sprintf("failed to remove existing app/logger test directory '%s': %v\n", testRootDir, err))
		} else {
			fmt.Printf("existing app/logger test dir successfully removed: '%s'\n", testRootDir)
		}
	} else {
		if !os.IsNotExist(err) {
			panic(fmt.Sprintf("os.Stat failed for app/logger test directory '%s': %v\n", testRootDir, err))
		}
	}
	// Create new testDir
	err = os.MkdirAll(testRootDir, 0775)
	if err != nil {
		panic(fmt.Sprintf("failed to create app/logger test directory '%s': %v\n", testRootDir, err))
	}

	fmt.Printf("--- created test dir for app/logger tests: '%s' ---\n", testRootDir)
}

func teardown(m *testing.M) {
	if removeTestRootDir {
		err := os.RemoveAll(testRootDir)
		if err != nil {
			fmt.Fprintf(os.Stderr,
				"failed to remove app/logger test directory: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("--- app/logger test directory successfully removed ---")
	} else {
		fmt.Printf("--- app/logger tests complete. You can remove the test directory manually if required: '%s'\n ---", testRootDir)
	}
}

// TEST SETUP END

func TestLoggerConfigParsed(t *testing.T) {

	data, err := os.ReadFile("test_data/TestLoggerConfig/c1.yaml")
	require.Equal(t, nil, err)
	testYamlConfig := string(data)

	var expectedTestConfig = &Config{
		LogFiles: []FileOutputConfig{
			{
				Path:            "logs/test.log",
				FilePermissions: 0660,
				DirPermissions:  0775,
			},
		},
		Console: []ConsoleOutputConfig{
			{
				Prettify: true,
			},
		},
	}

	parsed, err := ParseYamlConfig([]byte(testYamlConfig))
	require.Equal(t, nil, err)
	fmt.Printf("Parsed yaml config: %+v\n", *parsed)

	require.Equal(t, expectedTestConfig, parsed)
}

func TestAppLoggerBasicUsage(t *testing.T) {

	DefaultAppLoggerUnitName = fmt.Sprintf("logger_unit_%d", testId)
	testId++

	testDir := createSubDir("TestAppLoggerBasicUsage")
	configBytes, err := os.ReadFile("./test_data/TestAppLoggerBasicUsage.yaml")
	require.Equal(t, nil, err)

	config, err := ParseYamlConfig([]byte(configBytes))
	require.Equal(t, nil, err)

	for _, file := range config.LogFiles {
		// Prepend testDir path to the paths specified in the config
		file.Path = join(testDir, file.Path)
	}

	err = Create(config)
	require.Equal(t, nil, err, "app/logger must be created successfully")

	// Log dirs and files must exist
	for _, file := range config.LogFiles {
		fmt.Printf("dir: %q\n", filepath.Dir(file.Path))
		require.DirExists(t, filepath.Dir(file.Path))

		fmt.Printf("file: %q\n", file.Path)
		require.FileExists(t, file.Path)
	}

	l := Get()

	// Write some log records
	l.Debug().Msg("dummy debug message")

	err = Close()
	require.Equal(t, nil, err, "app/logger must close successfully")

}

func TestLogRotation(t *testing.T) {

	DefaultAppLoggerUnitName = fmt.Sprintf("logger_unit_%d", testId)
	testId++

	testDir := createSubDir("TestLogRotation")
	configBytes, err := os.ReadFile("./test_data/TestLogRotation.yaml")
	require.Equal(t, nil, err)

	config, err := ParseYamlConfig([]byte(configBytes))
	require.Equal(t, nil, err)

	for _, file := range config.LogFiles {
		// Prepend testDir path to the paths specified in the config
		file.Path = join(testDir, file.Path)
	}

	err = Create(config)
	require.Equal(t, nil, err, "app/logger must be created successfully")

	l := Get()

	// Write about 3.5MB of logs
	for x := 1; x <= 40000; x++ {
		l.Debug().Msgf("this is a dummy debug message to check log rotation #%d", x)
	}

	err = Close()
	require.Equal(t, nil, err, "app/logger must close successfully")

	// Log dirs and files must exist
	for _, file := range config.LogFiles {

		d := filepath.Dir(file.Path)
		require.DirExists(t, d)
		require.FileExists(t, file.Path)

		f, err := os.Open(d)
		require.Equal(t, nil, err, "rotated log directory must be open successfully")

		names, err := f.Readdirnames(-1)
		f.Close()
		require.Equal(t, nil, err)

		// Check files in the log dir:
		require.GreaterOrEqual(t, len(names), 3, "there must be at least 3 files in the rotated log directory")

		// The commented out test is not reliable because OS may create temporary files
		// in that directory.
		// require.LessOrEqual(t, len(names), 5, "there must be at most 5 files in the rotated log directory (two .log files may be still converting into .log.gz)")

		namesMatch := true
		for _, name := range names {
			if !(strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".log.gz")) {
				namesMatch = false
			}
		}
		require.Equal(t, true, namesMatch, "name of each file must be either 'rotated.log' or end with .log.gz")
	}

}

func TestTelegramNotifier(t *testing.T) {

	DefaultAppLoggerUnitName = fmt.Sprintf("logger_unit_%d", testId)
	testId++

	testDir := createSubDir("TestTelegramNotifier")
	configBytes, err := os.ReadFile("./test_data/TestTelegramNotifier.yaml")
	require.Equal(t, nil, err)

	config, err := ParseYamlConfig([]byte(configBytes))
	require.Equal(t, nil, err)

	for _, file := range config.LogFiles {
		// Prepend testDir path to the paths specified in the config
		file.Path = join(testDir, file.Path)
	}

	// Create and start telegram_notifier unit
	tgConfigBytes, err := os.ReadFile("./test_data/TelegramNotifierConfig.yaml")
	require.Equal(t, nil, err)

	// Create logger
	err = Create(config)
	require.Equal(t, nil, err, "logger must be created successfully")

	tgNotifierConfig, err := telegram_notifier.ParseYamlConfig([]byte(tgConfigBytes))
	require.Equal(t, nil, err, "telegram_notifier configuration must be parsed without errors")

	tgNotifierUnitName := fmt.Sprintf("telegram_notifier_%d", testId)
	tgNotifier, err := telegram_notifier.New(tgNotifierUnitName, tgNotifierConfig)
	require.Equal(t, nil, err, "new telegram_notifier instance must be created without errors")

	RegisterHooks(tgNotifier)

	tgNotifier.SetLogMessageTitleSuffix("logger_test")

	err = app.M.AddUnit(tgNotifier)
	require.Equal(t, nil, err, "telegram_notifier unit must be added successfully")
	_, err = app.M.Start(tgNotifierUnitName)
	require.Equal(t, nil, err, "telegram_notifier unit must start successfully")

	require.Equal(t, nil, err, "logger must be created successfully")

	l := Get()
	l.Trace().Msg("Test message 0.")
	l.Debug().Msg("Test message 1.")
	l.Info().Msgf("CI: looks like `igulib/app/logger` package test was successful, OS: %q.", runtime.GOOS)
	l.Warn().Msgf("Important: this is `igulib/app/logger` package test message, OS: %q.", runtime.GOOS)
	l.Error().Msg("Test message 4.")
	l.WithLevel(zerolog.FatalLevel).Msg("! Test message 5.")
	l.WithLevel(zerolog.PanicLevel).Msg("! Test message 6.")

	require.Equal(t, true, true)

	_, err = app.M.Pause(tgNotifierUnitName)
	require.Equal(t, nil, err, "telegram_notifier unit must pause successfully")

	_, err = app.M.Quit(tgNotifierUnitName)
	require.Equal(t, nil, err, "telegram_notifier unit must quit successfully")

	err = Close()
	require.Equal(t, nil, err, "logger_unit must close without errors")
}
