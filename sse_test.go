// Copyright 2022 Flamego. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sse

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flamego/flamego"
)

type mockResponseWriter struct {
	lock sync.Mutex
	*httptest.ResponseRecorder
}

func (m *mockResponseWriter) Write(buf []byte) (int, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.ResponseRecorder.Write(buf)
}

func (m *mockResponseWriter) Body() string {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.ResponseRecorder.Body.String()
}

func TestBind(t *testing.T) {
	f := flamego.NewWithLogger(&bytes.Buffer{})

	type object struct {
		Message string
	}

	f.Get("/normal",
		Bind(object{}),
		func(msg chan<- *object) {
			msg <- &object{Message: "Flamego"}
			// Sleep for the response flushed.
			time.Sleep(1 * time.Second)
		},
	)
	f.Get("/ping",
		Bind(
			object{},
			Options{
				300 * time.Millisecond, // Sleep for 1 second, so we can get 3 pings.
			},
		),
		func(msg chan<- *object) {
			msg <- &object{Message: "Flamego"}
			// Sleep for the response flushed.
			time.Sleep(1 * time.Second)
		},
	)
	f.Get("/ticker",
		Bind(
			object{},
			Options{
				100 * time.Millisecond,
			},
		),
		func(ctx flamego.Context, msg chan<- *object) {
			ticker := time.NewTicker(1 * time.Second)
			defer func() { ticker.Stop() }()

			for {
				select {
				case <-ticker.C:
					msg <- &object{Message: "Flamego"}
				case <-ctx.Request().Context().Done():
					return
				}
			}
		},
	)

	t.Run("normal", func(t *testing.T) {
		resp := &mockResponseWriter{
			ResponseRecorder: httptest.NewRecorder(),
		}
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
		assert.Equal(t, wantBody, resp.Body())
	})

	t.Run("ping", func(t *testing.T) {
		resp := &mockResponseWriter{
			ResponseRecorder: httptest.NewRecorder(),
		}
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
		assert.Equal(t, wantBody, resp.Body())
	})

	t.Run("close connection", func(t *testing.T) {
		server := httptest.NewServer(f)

		reqContext, cancel := context.WithCancel(context.Background())
		req, err := http.NewRequestWithContext(reqContext, http.MethodGet, server.URL+"/ticker", nil)
		require.NoError(t, err)

		// Close request connection after 1 second.
		go func() {
			time.Sleep(1 * time.Second)
			cancel()
		}()
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		err = resp.Body.Close()
		require.NoError(t, err)

		// Sleep for 3 seconds to wait for new responses that may be in a closed request.
		time.Sleep(3 * time.Second)
	})
}
