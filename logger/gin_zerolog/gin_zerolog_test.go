package gin_zerolog

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	c := &Config{}
	vc, err := c.Validate()
	require.Equal(t, nil, err)
	require.Equal(t, vc.DefaultLogLevel, zerolog.NoLevel)
	require.Equal(t, vc.RequestErrorLogLevel, zerolog.NoLevel)
	require.Equal(t, vc.InternalErrorLogLevel, zerolog.WarnLevel)
}

func TestNewMiddleware(t *testing.T) {
	// This is just a "smoke test",
	// for more tests see http_server_unit_test.go
	c := &Config{}
	lm, err := NewMiddleware(c)
	require.Equal(t, nil, err)
	require.NotEqualValues(t, nil, lm)
}
