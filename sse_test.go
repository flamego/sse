// Copyright 2022 Flamego. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sse

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/flamego/flamego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler(t *testing.T) {
	f := flamego.NewWithLogger(&bytes.Buffer{})

	type bind struct {
		Message string
	}

	f.Get("/normal", Handler(bind{}), func(msg chan<- *bind) {
		msg <- &bind{Message: "Flamego"}
		// Sleep for the response flushed.
		time.Sleep(1 * time.Second)
	})
	f.Get("/ping", Handler(bind{}, Options{
		300 * time.Millisecond, // Sleep for 1 second, so we can get 3 pings.
	}), func(msg chan<- *bind) {
		msg <- &bind{Message: "Flamego"}
		// Sleep for the response flushed.
		time.Sleep(1 * time.Second)
	})

	t.Run("normal", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/normal", nil)
		require.NoError(t, err)

		f.ServeHTTP(resp, req)

		assert.Equal(t, resp.Code, http.StatusOK)
		assert.Equal(t, "text/event-stream", resp.Header().Get("Content-Type"))
		assert.Equal(t, "no-cache", resp.Header().Get("Cache-Control"))
		assert.Equal(t, "keep-alive", resp.Header().Get("Connection"))
		assert.Equal(t, "no", resp.Header().Get("X-Accel-Buffering"))
		wantBody := `: ping

events: stream opened

data: {"Message":"Flamego"}

`
		assert.Equal(t, wantBody, resp.Body.String())
	})

	t.Run("ping", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/ping", nil)
		require.NoError(t, err)

		f.ServeHTTP(resp, req)

		assert.Equal(t, resp.Code, http.StatusOK)
		assert.Equal(t, "text/event-stream", resp.Header().Get("Content-Type"))
		assert.Equal(t, "no-cache", resp.Header().Get("Cache-Control"))
		assert.Equal(t, "keep-alive", resp.Header().Get("Connection"))
		assert.Equal(t, "no", resp.Header().Get("X-Accel-Buffering"))

		wantBody := `: ping

events: stream opened

data: {"Message":"Flamego"}

: ping

: ping

: ping

`
		assert.Equal(t, wantBody, resp.Body.String())
	})
}
