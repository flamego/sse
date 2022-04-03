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

const (
	defaultPingInterval = 10 * time.Second
)

type Options struct {
	// PingInterval is the time to wait between sending pings to the client.
	PingInterval time.Duration
}

type Connection struct {
	*Options

	// Sender is the channel used for sending out data to the client.
	// This channel gets mapped for the next handler to use with the right type
	// and is asynchronous unless the SendChannelBuffer is set to 0.
	Sender reflect.Value

	// context is the request context.
	context flamego.Context

	// ticker for pinging the client.
	ticker *time.Ticker
}

func Handler(bind interface{}, options ...*Options) flamego.Handler {
	return func(ctx flamego.Context, req *http.Request, resp http.ResponseWriter) {
		ctx.ResponseWriter().Header().Set("Content-Type", "text/event-stream")
		ctx.ResponseWriter().Header().Set("Cache-Control", "no-cache")
		ctx.ResponseWriter().Header().Set("Connection", "keep-alive")
		ctx.ResponseWriter().Header().Set("X-Accel-Buffering", "no")

		sse := &Connection{
			Options: newOptions(options),
			context: ctx,
			Sender:  makeChanOfType(reflect.TypeOf(bind), 1),
		}
		ctx.Set(reflect.ChanOf(reflect.SendDir, sse.Sender.Type().Elem()), sse.Sender)

		go sse.handle()

		ctx.Next()
	}
}

// newOptions creates new default options and assigns any given options.
func newOptions(options []*Options) *Options {
	if len(options) == 0 {
		return &Options{
			PingInterval: defaultPingInterval,
		}
	}
	return options[0]
}

// startTicker starts the ticker used for pinging the client.
func (c *Connection) startTicker() {
	c.ticker = time.NewTicker(c.PingInterval)
}

// stopTicker stop the ticker used for pinging the client.
func (c *Connection) stopTicker() {
	c.ticker.Stop()
}

func (c *Connection) write(msg string) {
	_, _ = c.context.ResponseWriter().Write([]byte(msg))
}

func (c *Connection) flush() {
	c.context.ResponseWriter().Flush()
}

const (
	senderSend = iota
	tickerTick
	timeout
)

func (c *Connection) handle() {
	c.startTicker()
	defer func() { c.stopTicker() }()

	c.write(": ping\n\n")
	c.write("events: stream opened\n\n")
	c.flush()

	cases := make([]reflect.SelectCase, 3)
	cases[senderSend] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: c.Sender, Send: reflect.ValueOf(nil)}
	cases[tickerTick] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(c.ticker.C), Send: reflect.ValueOf(nil)}
	cases[timeout] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(time.After(time.Hour)), Send: reflect.ValueOf(nil)}

L:
	for {
		chosen, message, ok := reflect.Select(cases)
		switch chosen {

		case senderSend:
			if !ok {
				// Sender channel has been closed.
				return
			}

			c.write("data: ")
			evt, _ := json.Marshal(message.Interface())
			c.write(string(evt))
			c.write("\n\n")
			c.flush()

		case tickerTick:
			c.write(": ping\n\n")
			c.flush()

		case timeout:
			c.write("events: stream timeout\n\n")
			c.flush()
			break L
		}
	}

	c.write("event: error\ndata: eof\n\n")
	c.flush()
	c.write("events: stream closed")
	c.flush()
}

// makeChanOfType create a chan of the given type as a reflect.Value.
func makeChanOfType(typ reflect.Type, chanBuffer int) reflect.Value {
	return reflect.MakeChan(reflect.ChanOf(reflect.BothDir, reflect.PtrTo(typ)), chanBuffer)
}
