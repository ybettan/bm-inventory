package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockTransport struct {
	mock.Mock
}

func TestMiddleware(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		username string
	}{
		{
			name:     "username exist",
			username: "eran",
		},
		{
			name:     "username empty",
			username: "",
		},
	}

	for _, tt := range tests {
		headerKey := "username"
		t.Run(tt.name, func(t *testing.T) {
			// create a handler to use as "next" which will verify the request
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				val := r.Context().Value(ContextUsernameKey)
				assert.NotNil(t, val)
				valStr, ok := val.(string)
				if !ok {
					t.Error("not string")
				}

				assert.Equal(t, valStr, tt.username)

			})

			// create the handler to test, using our custom "next" handler
			h := GetUserMiddleware(nextHandler)

			// create a mock request to use
			req := httptest.NewRequest("GET", "http://testing", nil)
			if tt.username != "" {
				req.Header.Set(headerKey, tt.username)
			}

			// call the handler using a mock response recorder (we'll not use that anyway)
			h.ServeHTTP(httptest.NewRecorder(), req)
		})
	}
}
