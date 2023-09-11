package http_server_unit

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/igulib/app"
	"github.com/igulib/app/logger"
	"github.com/rs/zerolog"
)

// HttpServerUnit implements app.IUnit
type HttpServerUnit struct {
	config               *validatedConfig
	unitRunner           *app.UnitLifecycleRunner
	unitAvailability     app.UnitAvailability
	unitAvailabilityLock sync.Mutex
	logger               zerolog.Logger

	rootHandler http.Handler

	ginServiceRunning atomic.Bool
	srv               *http.Server
	srvStartComplete  chan error
}

func New(unitName string, c *Config, rootHandler http.Handler) (*HttpServerUnit, error) {
	if c == nil {
		return nil, errors.New("http_server_unit config must not be nil")
	}
	vc, err := c.ValidateConfig()
	if err != nil {
		return nil, fmt.Errorf("http_server_unit failed to validate config: %w", err)
	}

	u := &HttpServerUnit{
		config:      vc,
		unitRunner:  app.NewUnitLifecycleRunner(unitName),
		logger:      logger.Get().With().Str("unit", unitName).Logger(),
		rootHandler: rootHandler,
	}

	u.unitRunner.SetOwner(u)
	return u, nil
}

// AddNew creates HttpServerUnit with specified configuration and
// adds it to the igulib/app.M
func AddNew(unitName string, c *Config, rootHandler http.Handler) (*HttpServerUnit, error) {
	u, err := New(unitName, c, rootHandler)
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
		time.Duration(u.config.ShutdownPeriodMillis)*time.Millisecond)
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

	// Specify listen host and port
	hostAndPort := fmt.Sprintf("%s:%s", u.config.Host,
		u.config.Port)
	u.srv = &http.Server{
		Addr:    hostAndPort,
		Handler: u.rootHandler,
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
			// Server was shut down (not an error)
		} else {
			// Unexpected error
			u.unitAvailabilityLock.Lock()
			u.unitAvailability = app.UNotAvailable
			u.unitAvailabilityLock.Unlock()
			logger.Error().Msgf("(HttpServerUnit) Serve error: %s\n", err)
		}
	}
	u.ginServiceRunning.Store(false)
}
