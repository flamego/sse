// Copyright 2022 Flamego. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sse

import (
	"encoding/json"
	"net/http"
	"reflect"
	"time"

	"github.com/flamego/flamego"
)

// Options contains options for the sse.Handler middleware.
type Options struct {
	// PingInterval is the time to wait between sending pings to the client.
	// Default is 10 seconds.
	PingInterval time.Duration
}

type connection struct {
	Options

	// sender is the channel used for sending out data to the client.
	// This channel gets mapped for the next handler to use with the right type
	// and is asynchronous unless the SendChannelBuffer is set to 0.
	sender reflect.Value

	// context is the request context.
	context flamego.Context

	// ticker for pinging the client.
	ticker *time.Ticker
}

func Handler(bind interface{}, opts ...Options) flamego.Handler {
	return func(ctx flamego.Context, req *http.Request, resp http.ResponseWriter) {
		ctx.ResponseWriter().Header().Set("Content-Type", "text/event-stream")
		ctx.ResponseWriter().Header().Set("Cache-Control", "no-cache")
		ctx.ResponseWriter().Header().Set("Connection", "keep-alive")
		ctx.ResponseWriter().Header().Set("X-Accel-Buffering", "no")

		sse := &connection{
			Options: newOptions(opts),
			context: ctx,
			// create a chan of the given type as a reflect.Value.
			sender: reflect.MakeChan(reflect.ChanOf(reflect.BothDir, reflect.PtrTo(reflect.TypeOf(bind))), 1),
		}
		ctx.Set(reflect.ChanOf(reflect.SendDir, sse.sender.Type().Elem()), sse.sender)

		go sse.handle()

		ctx.Next()
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

func (c *connection) write(msg string) {
	_, _ = c.context.ResponseWriter().Write([]byte(msg))
}

func (c *connection) flush() {
	c.context.ResponseWriter().Flush()
}

const (
	senderSend = iota
	tickerTick
	timeout
)

func (c *connection) handle() {
	// Starts the ticker used for pinging the client.
	c.ticker = time.NewTicker(c.PingInterval)
	defer func() { c.ticker.Stop() }()

	c.write(": ping\n\n")
	c.write("events: stream opened\n\n")
	c.flush()

	cases := make([]reflect.SelectCase, 3)
	cases[senderSend] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: c.sender, Send: reflect.ValueOf(nil)}
	cases[tickerTick] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(c.ticker.C), Send: reflect.ValueOf(nil)}
	cases[timeout] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(time.After(time.Hour)), Send: reflect.ValueOf(nil)}

loop:
	for {
		chosen, message, ok := reflect.Select(cases)
		switch chosen {
		case senderSend:
			if !ok {
				// Sender channel has been closed.
				return
			}

			c.write("data: ")
			evt, err := json.Marshal(message.Interface())
			if err != nil {
				// Panic if the message can not be marshaled.
				panic("sse: " + err.Error())
			}
			c.write(string(evt))
			c.write("\n\n")
			c.flush()

		case tickerTick:
			c.write(": ping\n\n")
			c.flush()

		case timeout:
			c.write("events: stream timeout\n\n")
			c.flush()
			break loop
		}
	}

	c.write("events: error\ndata: eof\n\n")
	c.flush()
	c.write("events: stream closed")
	c.flush()
}
