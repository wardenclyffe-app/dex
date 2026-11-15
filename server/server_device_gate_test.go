package server

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dexidp/dex/server/oauth2"
)

func TestServerDeviceEndpointsGatedByGrant(t *testing.T) {
	devicePaths := []string{
		"/device",
		"/device/auth/verify_code",
		"/device/code",
		oauth2.DeviceCallbackURI,
	}

	t.Run("not routed when device_code grant is disallowed", func(t *testing.T) {
		httpServer, _ := newTestServer(t, func(c *Config) {
			c.AllowedGrantTypes = []string{oauth2.GrantTypeAuthorizationCode, oauth2.GrantTypeRefreshToken}
		})
		defer httpServer.Close()

		for _, p := range devicePaths {
			resp, err := http.Get(httpServer.URL + p)
			require.NoError(t, err)
			_ = resp.Body.Close()
			require.Equal(t, http.StatusNotFound, resp.StatusCode, "GET %s must be 404", p)
		}
	})

	t.Run("routed when device_code grant is allowed", func(t *testing.T) {
		httpServer, _ := newTestServer(t, nil)
		defer httpServer.Close()

		for _, p := range devicePaths {
			resp, err := http.Get(httpServer.URL + p)
			require.NoError(t, err)
			_ = resp.Body.Close()
			require.NotEqual(t, http.StatusNotFound, resp.StatusCode, "GET %s must be routed", p)
		}
	})
}
