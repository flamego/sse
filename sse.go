// Copyright 2022 Flamego. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sse

import (
	"encoding/json"
	"log"
	"reflect"
	"time"

	"github.com/flamego/flamego"
)

// Options contains options for the sse.Bind middleware.
type Options struct {
	// PingInterval is the time internal to wait between sending pings to the
	// client. Default is 10 seconds.
	PingInterval time.Duration
}

type connection struct {
	Options

	// sender is the channel used for sending out data to the client. This channel
	// gets mapped for the next handler to use with the right type and is
	// asynchronous unless the SendChannelBuffer is set to 0.
	sender reflect.Value
}

// Bind returns a middleware handler that uses the given bound object as the
// date type for sending events.
func Bind(obj interface{}, opts ...Options) flamego.Handler {
	return func(c flamego.Context, log *log.Logger) {
		c.ResponseWriter().Header().Set("Content-Type", "text/event-stream")
		c.ResponseWriter().Header().Set("Cache-Control", "no-cache")
		c.ResponseWriter().Header().Set("Connection", "keep-alive")
		c.ResponseWriter().Header().Set("X-Accel-Buffering", "no")

		sse := &connection{
			Options: newOptions(opts),
			// Create a chan of the given type as a reflect.Value.
			sender: reflect.MakeChan(reflect.ChanOf(reflect.BothDir, reflect.PtrTo(reflect.TypeOf(obj))), 0),
		}
		c.Set(reflect.ChanOf(reflect.SendDir, sse.sender.Type().Elem()), sse.sender)

		// stopCh is closed when the next handler returns, signaling handle()
		// to stop writing to the ResponseWriter.
		stopCh := make(chan struct{})
		// doneCh is closed when handle() exits, allowing the handler to wait
		// for the goroutine to fully stop before returning.
		doneCh := make(chan struct{})

		go func() {
			defer close(doneCh)
			sse.handle(log, c, stopCh)
		}()

		// Call the next handler(s) in the chain. When they return, signal
		// the handle goroutine to stop, then wait for it to finish before
		// returning control to the HTTP server.
		c.Next()
		close(stopCh)
		<-doneCh
	}
}

// newOptions creates new default options and assigns any given options.
func newOptions(opts []Options) Options {
	if len(opts) == 0 {
		return Options{
			PingInterval: 10 * time.Second,
		}
	}
	return opts[0]
}

func (c *connection) handle(log *log.Logger, ctx flamego.Context, stopCh <-chan struct{}) {
	w := ctx.ResponseWriter()
	ticker := time.NewTicker(c.PingInterval)
	defer func() { ticker.Stop() }()

	write := func(msg string) {
		_, err := w.Write([]byte(msg))
		if err != nil {
			log.Printf("sse: failed to write message: %v", err)
		}
	}

	write(": ping\n\n")
	write("events: stream opened\n\n")
	w.Flush()

	const (
		senderSend = iota
		tickerTick
		timeout
		closed
		stopped
	)
	cases := make([]reflect.SelectCase, 5)
	cases[senderSend] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: c.sender, Send: reflect.ValueOf(nil)}
	cases[tickerTick] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ticker.C), Send: reflect.ValueOf(nil)}
	cases[timeout] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(time.After(time.Hour)), Send: reflect.ValueOf(nil)}
	cases[closed] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Request().Context().Done()), Send: reflect.ValueOf(nil)}
	cases[stopped] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(stopCh), Send: reflect.ValueOf(nil)}

loop:
	for {
		chosen, message, ok := reflect.Select(cases)
		switch chosen {
		case senderSend:
			if !ok {
				// Sender channel has been closed.
				return
			}

			write("data: ")
			evt, err := json.Marshal(message.Interface())
			if err != nil {
				log.Printf("sse: failed to marshal message: %v", err)
				continue
			}
			write(string(evt))
			write("\n\n")
			w.Flush()

		case tickerTick:
			write(": ping\n\n")
			w.Flush()

		case timeout:
			write("events: stream timeout\n\n")
			w.Flush()
			break loop

		case closed:
			return

		case stopped:
			return
		}
	}

	write("events: error\ndata: eof\n\n")
	w.Flush()
	write("events: stream closed")
	w.Flush()
}
