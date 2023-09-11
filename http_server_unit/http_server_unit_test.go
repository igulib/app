package http_server_unit

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/igulib/app"
	"github.com/igulib/app/logger/gin_zerolog"
	"github.com/stretchr/testify/require"
)

// TEST SETUP BEGIN

// Temporary directory for all http_server_unit tests.
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
	fmt.Println("--- http_server_unit tests setup ---")
	testRootDir = join(os.TempDir(), "http_server_unit_tests")
	// Remove old testRootDir if exists
	_, err := os.Stat(testRootDir)
	if err == nil {
		err = os.RemoveAll(testRootDir)
		if err != nil {
			panic(fmt.Sprintf("failed to remove existing http_server_unit test directory '%s': %v\n", testRootDir, err))
		} else {
			fmt.Printf("existing http_server_unit test dir successfully removed: '%s'\n", testRootDir)
		}
	} else {
		if !os.IsNotExist(err) {
			panic(fmt.Sprintf("os.Stat failed for http_server_unit test directory '%s': %v\n", testRootDir, err))
		}
	}
	// Create new testDir
	err = os.MkdirAll(testRootDir, 0775)
	if err != nil {
		panic(fmt.Sprintf("failed to create http_server_unit test directory '%s': %v\n", testRootDir, err))
	}

	fmt.Printf("--- created test dir for http_server_unit tests: '%s' ---\n", testRootDir)
}

func teardown(m *testing.M) {
	if removeTestRootDir {
		err := os.RemoveAll(testRootDir)
		if err != nil {
			fmt.Fprintf(os.Stderr,
				"failed to remove http_server_unit test directory: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("--- http_server_unit test directory successfully removed ---")
	} else {
		fmt.Printf("--- http_server_unit tests complete. You can remove the test directory manually if required: '%s'\n ---", testRootDir)
	}
}

// TEST SETUP END

func doTestGetHttpRequest(port string) (int, error) {
	apiUrl := "http://localhost:" + port
	resource := "/test/"
	data := url.Values{}

	u, err := url.ParseRequestURI(apiUrl)
	if err != nil {
		return 0, err
	}
	u.Path = resource
	urlStr := u.String() // "http://localhost:8778/test/"

	client := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, urlStr, strings.NewReader(data.Encode()))
	if err != nil {
		return 0, err
	}
	// r.Header.Add("Authorization", "auth_token=\"XXXXXXX\"")
	// r.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if resp != nil {
		return resp.StatusCode, err
	}
	return 0, err
}

func TestBasicUsage(t *testing.T) {

	configBytes, err := os.ReadFile("./test_data/TestBasicUsage.yaml")
	require.Equal(t, nil, err)

	config, err := ParseYamlConfig([]byte(configBytes))
	require.Equal(t, nil, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/test/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "Test was successful.")
	})

	unitName := "http_server_unit_1"
	u, err := AddNew(unitName, config, mux)
	require.Equal(t, nil, err, "http_server_unit must be created and added successfully")

	require.Equal(t, app.UNotAvailable, u.UnitAvailability())

	_, err = app.M.Start(unitName)
	require.Equal(t, nil, err, "http_server_unit must start successfully")
	require.Equal(t, app.UAvailable, u.UnitAvailability())

	statusCode, err := doTestGetHttpRequest(config.Port)

	require.Equal(t, nil, err)
	require.Equal(t, 200, statusCode)

	_, err = app.M.Pause(unitName)
	require.Equal(t, nil, err, "http_server_unit must pause successfully")
	require.Equal(t, app.UTemporarilyUnavailable, u.UnitAvailability())

	_, err = app.M.Quit(unitName)
	require.Equal(t, nil, err, "http_server_unit must quit successfully")
	require.Equal(t, app.UNotAvailable, u.UnitAvailability())
}

func TestHttpServerUnitWithGinGonicHandler(t *testing.T) {

	configBytes, err := os.ReadFile("./test_data/TestWithGinGonic.yaml")
	require.Equal(t, nil, err)

	config, err := ParseYamlConfig([]byte(configBytes))
	require.Equal(t, nil, err)

	// Gin router
	gin.SetMode(gin.ReleaseMode)

	r := gin.New() // empty engine instead of the default one
	ginLoggerConfig := &gin_zerolog.Config{}
	ginLoggerMiddleware, err := gin_zerolog.NewMiddleware(ginLoggerConfig)
	require.Equal(t, nil, err)
	r.Use(ginLoggerMiddleware) // add logger middleware
	r.Use(gin.Recovery())      // add the default recovery middleware

	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "Test was successful.")
	})

	// Custom page 404
	r.NoRoute(func(c *gin.Context) {
		c.String(http.StatusNotFound, "The page you requested does not exist.")
	})

	unitName := "http_server_unit_2"
	u, err := AddNew(unitName, config, r)
	require.Equal(t, nil, err, "http_server_unit must be created and added successfully")

	require.Equal(t, app.UNotAvailable, u.UnitAvailability())

	_, err = app.M.Start(unitName)
	require.Equal(t, nil, err, "http_server_unit must start successfully")
	require.Equal(t, app.UAvailable, u.UnitAvailability())

	statusCode, err := doTestGetHttpRequest(config.Port)

	require.Equal(t, nil, err)
	require.Equal(t, 200, statusCode)

	_, err = app.M.Pause(unitName)
	require.Equal(t, nil, err, "http_server_unit must pause successfully")
	require.Equal(t, app.UTemporarilyUnavailable, u.UnitAvailability())

	_, err = app.M.Quit(unitName)
	require.Equal(t, nil, err, "http_server_unit must quit successfully")
	require.Equal(t, app.UNotAvailable, u.UnitAvailability())
}
