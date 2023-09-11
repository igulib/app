// Based on https://github.com/gin-contrib/logger
// and https://betterstack.com/community/guides/logging/zerolog/

package gin_zerolog

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/igulib/app/logger"
	"github.com/rs/zerolog"
)

type Config struct {
	SkipPaths      []string       `yaml:"skip_paths"`
	SkipPathRegexp *regexp.Regexp `yaml:"skip_path_regexp"`

	// DefaultLogLevel defines the log level for http response
	// codes in range [0..399].
	//
	// Possible values: ["trace", "debug", "info", "warn", "error", "fatal", "panic"]
	// or empty string for NoLevel.
	DefaultLogLevel string `yaml:"default_log_level"`

	// RequestErrorLogLevel defines the log level for http response
	// codes in range [400..499].
	//
	// Possible values: ["trace", "debug", "info", "warn", "error", "fatal", "panic"]
	// or empty string for NoLevel.
	RequestErrorLogLevel string `yaml:"request_error_log_level"`

	// InternalErrorLogLevel defines the log level for http response
	// codes 500 and above.
	//
	// Possible values: ["trace", "debug", "info", "warn", "error", "fatal", "panic"]
	// or empty string for NoLevel. The default is zerolog.WarnLevel.
	InternalErrorLogLevel string `yaml:"internal_error_log_level"`
}

func (c *Config) Validate() (*validatedConfig, error) {
	vc := &validatedConfig{
		SkipPaths:      make([]string, 0),
		SkipPathRegexp: c.SkipPathRegexp,
	}

	if c.InternalErrorLogLevel == "" {
		c.InternalErrorLogLevel = "warn"
	}

	var err error
	if vc.DefaultLogLevel, err = zerolog.ParseLevel(c.DefaultLogLevel); err != nil {
		return vc, fmt.Errorf("failed to parse gin_zerolog.Config.DefaultLogLevel: %w", err)
	}

	if vc.InternalErrorLogLevel, err = zerolog.ParseLevel(c.InternalErrorLogLevel); err != nil {
		return vc, fmt.Errorf("failed to parse gin_zerolog.Config.InternalErrorLogLevel: %w", err)
	}

	if vc.RequestErrorLogLevel, err = zerolog.ParseLevel(c.RequestErrorLogLevel); err != nil {
		return vc, fmt.Errorf("failed to parse gin_zerolog.Config.RequestErrorLogLevel: %w", err)
	}

	return vc, nil
}

type validatedConfig struct {
	SkipPaths      []string
	SkipPathRegexp *regexp.Regexp

	// DefaultLogLevel defines the log level for http response
	// codes in range [0..399].
	DefaultLogLevel zerolog.Level

	// RequestErrorLogLevel defines the log level for http response
	// codes in range [400..499].
	RequestErrorLogLevel zerolog.Level

	// InternalErrorLogLevel defines the log level for http response
	// codes 500 and above.
	InternalErrorLogLevel zerolog.Level
}

var (
	noVal        = struct{}{}
	ErrNilConfig = errors.New("app/logger/gin_zerolog config must not be nil")
)

// NewMiddleware allows to use app/logger instead of the default
// gin-gonic logger. It creates a new gin middleware that can be used by gin.
// Usage example:
//
//	r := gin.New() // empty engine instead of the default one
//	ginZerologConfig := &gin_zerolog.Config{}
//	ginZerologMiddleware, err := gin_zerolog.NewMiddleware(ginZerologConfig)
//	if err != nil {
//	  panic("failed to create gin_zerolog middleware")
//	}
//	r.Use(ginZerologMiddleware)
//	r.Use(gin.Recovery()) // add the default recovery middleware
func NewMiddleware(rawConfig *Config) (gin.HandlerFunc, error) {
	if rawConfig == nil {
		return nil, ErrNilConfig
	}
	config, err := rawConfig.Validate()
	if err != nil {
		return nil, err
	}

	var skip map[string]struct{}
	if length := len(config.SkipPaths); length > 0 {
		skip = make(map[string]struct{}, length)
		for _, path := range config.SkipPaths {
			skip[path] = noVal
		}
	}

	return func(c *gin.Context) {

		l := *logger.Get()
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery
		if raw != "" {
			path = path + "?" + raw
		}

		c.Next()
		track := true

		if _, ok := skip[path]; ok {
			track = false
		}

		if track &&
			config.SkipPathRegexp != nil &&
			config.SkipPathRegexp.MatchString(path) {
			track = false
		}

		if track {
			// end := time.Now()
			// end = end.UTC()
			// elapsedMs := end.Sub(start)
			elapsedMs := time.Since(start).Milliseconds()

			l = l.With().
				Int("status", c.Writer.Status()).
				Str("method", c.Request.Method).
				Str("path", c.Request.URL.Path).
				Str("ip", c.ClientIP()).
				Int64("elapsed_ms", elapsedMs).
				// Dur("latency", latency).
				Str("user_agent", c.Request.UserAgent()).Logger()

			msg := "Request"
			if len(c.Errors) > 0 {
				msg = c.Errors.String()
			}

			switch {
			case c.Writer.Status() >= http.StatusBadRequest && c.Writer.Status() < http.StatusInternalServerError:
				{
					l.WithLevel(config.RequestErrorLogLevel).
						Msg(msg)
				}
			case c.Writer.Status() >= http.StatusInternalServerError:
				{
					l.WithLevel(config.InternalErrorLogLevel).
						Msg(msg)
				}
			default:
				l.WithLevel(config.DefaultLogLevel).
					Msg(msg)
			}
		}
	}, nil
}
