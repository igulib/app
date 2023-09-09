package http_server_unit

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/igulib/app"
	"github.com/igulib/app/logger"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// HttpServerUnit implements app.IUnit
type HttpServerUnit struct {
	config               *Config
	unitRunner           *app.UnitLifecycleRunner
	unitAvailability     app.UnitAvailability
	unitAvailabilityLock sync.Mutex
	logger               zerolog.Logger

	ginServiceRunning atomic.Bool
	srv               *http.Server
	srvStartComplete  chan error
}

func New(unitName string, c *Config) (*HttpServerUnit, error) {
	if err := c.ValidateConfig(); err != nil {
		return nil, err
	}

	u := &HttpServerUnit{
		config:     c,
		unitRunner: app.NewUnitLifecycleRunner(unitName),
		logger:     logger.Get().With().Str("unit", unitName).Logger(),
	}

	u.unitRunner.SetOwner(u)
	return u, nil
}

// AddNew creates HttpServerUnit with specified configuration and
// adds it to the igulib/app.M
func AddNew(unitName string, c *Config) (*HttpServerUnit, error) {
	u, err := New(unitName, c)
	if err != nil {
		return u, err
	}
	return u, app.M.AddUnit(u)
}

// UnitAvailability implements app.IUnit.
func (u *HttpServerUnit) UnitAvailability() app.UnitAvailability {
	var a app.UnitAvailability
	u.unitAvailabilityLock.Lock()
	a = u.unitAvailability
	u.unitAvailabilityLock.Unlock()
	return a
}

// UnitStart implements app.IUnit.
func (u *HttpServerUnit) UnitStart() app.UnitOperationResult {
	swapped := u.ginServiceRunning.CompareAndSwap(false, true)
	var err error
	if swapped {
		u.srvStartComplete = make(chan error)
		go u.runGinService()
		err = <-u.srvStartComplete

		// TODO: remove!
		// There is no straightforward and easy
		// way to know when the http server starts,
		// so use a reasonable delay.
		// time.Sleep(200 * time.Millisecond)
	}

	r := app.UnitOperationResult{
		OK:              err == nil,
		CollateralError: err,
	}
	return r
}

// UnitPause implements app.IUnit.
func (u *HttpServerUnit) UnitPause() app.UnitOperationResult {
	u.unitAvailabilityLock.Lock()
	u.unitAvailability = app.UTemporarilyUnavailable
	u.unitAvailabilityLock.Unlock()

	r := app.UnitOperationResult{
		OK: true,
	}
	return r
}

// UnitQuit implements app.IUnit.
func (u *HttpServerUnit) UnitQuit() app.UnitOperationResult {
	logger := u.logger

	u.unitAvailabilityLock.Lock()
	u.unitAvailability = app.UNotAvailable
	u.unitAvailabilityLock.Unlock()

	// Create a context with timeout equal to config.ShutdownGracePeriod
	ctxTimeout, cancel := context.WithTimeout(context.Background(),
		time.Duration(u.config.validatedShutdownPeriodMillis)*time.Millisecond)
	defer cancel()

	r := app.UnitOperationResult{}
	// Shutdown the server with grace period defined by ctxTimeout;
	// This blocks until either server shuts down or timeout occurs.
	if u.srv != nil {
		if err := u.srv.Shutdown(ctxTimeout); err != nil {
			r.CollateralError = err
			logger.Warn().Msgf("(HttpServerUnit) failed to shut down gracefully, forcing shutdown: %v", err)
		} else {
			r.OK = true
			logger.Debug().Msg("(HttpServerUnit) was shut down successfully.")
		}
	}

	return r
}

// UnitRunner implements app.IUnit.
func (u *HttpServerUnit) UnitRunner() *app.UnitLifecycleRunner {
	return u.unitRunner
}

func (u *HttpServerUnit) runGinService() {
	logger := u.logger

	gin.SetMode(gin.ReleaseMode)

	r := gin.New()        // empty engine instead of the default one
	r.Use(GinSetLogger()) // adds our new middleware
	r.Use(gin.Recovery()) // adds the default recovery middleware

	r.Static("/public", u.config.validatedAssetsDir)

	r.GET("favicon.ico", func(c *gin.Context) {
		defaultIcon := path.Join(u.config.validatedAssetsDir, "favicon.ico")
		file, _ := os.ReadFile(defaultIcon)
		c.Data(
			http.StatusOK,
			"image/x-icon",
			file,
		)
	})

	r.GET("/quit", func(c *gin.Context) {
		c.String(http.StatusOK, "Server was quit successfully.")
		_ = app.InitiateShutdown()
		time.Sleep(1000 * time.Millisecond)
	})

	r.GET("/notify-telegram", func(c *gin.Context) {
		msg, _ := c.GetQuery("msg")
		if msg == "" {
			msg = "Greetings from gin_server!"
		}
		log.Info().Msgf("Important: %s", msg)
		c.String(http.StatusOK, "Notification message sent.")
	})

	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "Test was successful.")
	})

	// Custom page 404
	r.NoRoute(func(c *gin.Context) {
		c.String(http.StatusNotFound, "The page you requested does not exist.")
		// c.Redirect(http.StatusFound, "/error/")
	})

	// Specify listen host and port
	hostAndPort := fmt.Sprintf("%s:%s", u.config.validatedHost,
		u.config.validatedPort)
	u.srv = &http.Server{
		Addr:    hostAndPort,
		Handler: r,
	}

	u.logger.Info().Msgf("(HttpServerUnit) listening on %s.", hostAndPort)

	u.unitAvailabilityLock.Lock()
	u.unitAvailability = app.UAvailable
	u.unitAvailabilityLock.Unlock()

	// net.Listen doesn't block
	listener, err := net.Listen("tcp", hostAndPort)
	u.srvStartComplete <- err
	close(u.srvStartComplete)

	if err := u.srv.Serve(listener); err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			// ErrServerClosed means server is shutting down
		} else {
			// Unexpected error
			u.unitAvailabilityLock.Lock()
			u.unitAvailability = app.UNotAvailable
			u.unitAvailabilityLock.Unlock()
			logger.Error().Msgf("(HttpServerUnit) Serve error: %s\n", err)
		}
	}

	// if err := u.srv.ListenAndServe(); err != nil {
	// 	if errors.Is(err, http.ErrServerClosed) {
	// 		// ErrServerClosed means server is shutting down
	// 	} else {
	// 		// Unexpected error
	// 		u.unitAvailabilityLock.Lock()
	// 		u.unitAvailability = app.UNotAvailable
	// 		u.unitAvailabilityLock.Unlock()
	// 		logger.Error().Msgf("(HttpServerUnit) ListenAndServe error: %s\n", err)
	// 	}
	// }

	u.ginServiceRunning.Store(false)
}
