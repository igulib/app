// based on https://github.com/gin-contrib/logger
// and https://betterstack.com/community/guides/logging/zerolog/

package http_server_unit

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/igulib/app/logger"
	"github.com/rs/zerolog"
)

var (
	noVal = struct{}{}
)

// GinSetLogger initializes the logging middleware.
func GinSetLogger() gin.HandlerFunc {

	// TODO: add skip path capability

	// var skip map[string]struct{}
	// if length := len(cfg.skipPath); length > 0 {
	// 	skip = make(map[string]struct{}, length)
	// 	for _, path := range cfg.skipPath {
	// 		skip[path] = struct{}{}
	// 	}
	// }

	var skip = map[string]struct{}{
		"/skip":       noVal,
		"/other-path": noVal,
	}

	return func(c *gin.Context) {
		// l := zerolog.New(cfg.output).
		// 	Output(
		// 		zerolog.ConsoleWriter{
		// 			Out:     cfg.output,
		// 			NoColor: !isTerm,
		// 		},
		// 	).
		// 	With().
		// 	Timestamp().
		// 	Logger()

		// if cfg.logger != nil {
		// 	l = cfg.logger(c, l)
		// }
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

		// if track &&
		// 	cfg.skipPathRegexp != nil &&
		// 	cfg.skipPathRegexp.MatchString(path) {
		// 	track = false
		// }

		if track {
			end := time.Now()
			end = end.UTC()

			latency := end.Sub(start)

			l = l.With().
				Int("status", c.Writer.Status()).
				Str("method", c.Request.Method).
				Str("path", c.Request.URL.Path).
				Str("ip", c.ClientIP()).
				Dur("latency", latency).
				Str("user_agent", c.Request.UserAgent()).Logger()

			msg := "Request"
			if len(c.Errors) > 0 {
				msg = c.Errors.String()
			}

			switch {
			case c.Writer.Status() >= http.StatusBadRequest && c.Writer.Status() < http.StatusInternalServerError:
				{
					l.WithLevel(zerolog.ErrorLevel).
						Msg(msg)
				}
			case c.Writer.Status() >= http.StatusInternalServerError:
				{
					l.WithLevel(zerolog.ErrorLevel).
						Msg(msg)
				}
			default:
				l.WithLevel(zerolog.DebugLevel).
					Msg(msg)
			}
		}
	}
}
