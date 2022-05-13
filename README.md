# sse

[![GitHub Workflow Status](https://img.shields.io/github/workflow/status/flamego/sse/Go?logo=github&style=for-the-badge)](https://github.com/flamego/sse/actions?query=workflow%3AGo)
[![Codecov](https://img.shields.io/codecov/c/gh/flamego/sse?logo=codecov&style=for-the-badge)](https://app.codecov.io/gh/flamego/sse)
[![GoDoc](https://img.shields.io/badge/GoDoc-Reference-blue?style=for-the-badge&logo=go)](https://pkg.go.dev/github.com/flamego/sse?tab=doc)
[![Sourcegraph](https://img.shields.io/badge/view%20on-Sourcegraph-brightgreen.svg?style=for-the-badge&logo=sourcegraph)](https://sourcegraph.com/github.com/flamego/sse)

Package sse is a middleware that provides Server-Sent Events for [Flamego](https://github.com/flamego/flamego).

## Installation

The minimum requirement of Go is **1.16**.

	go get github.com/flamego/sse

## Getting started

```html
<!-- templates/index.html -->
<script src="https://cdn.jsdelivr.net/npm/way-js@0.2.1/dist/way.js"></script>
<p><b><span way-data="data"></span></b>[<span way-data="published-at"></span>]</p>
<script>
  let es = new EventSource("/bulletin");
  es.onmessage = (evt) => {
    let bulletin = JSON.parse(evt.data);
    way.set('data', bulletin.Data)
    way.set('published-at', new Date(bulletin.PublishedAt).toLocaleString())
  };
</script>
```

```go
package main

import (
	"math/rand"
	"net/http"
	"time"

	"github.com/flamego/flamego"
	"github.com/flamego/sse"
	"github.com/flamego/template"
)

var bulletins = []string{"Hello Flamego!", "Flamingo? No, Flamego!", "Most powerful routing syntax", "Slim core but limitless extensibility"}

type bulletin struct {
	Data        string
	PublishedAt time.Time
}

func main() {
	f := flamego.Classic()
	f.Use(template.Templater(), flamego.Renderer())

	f.Get("/", func(ctx flamego.Context, t template.Template) {
		t.HTML(http.StatusOK, "index")
	})

	f.Get("/bulletin", sse.Bind(bulletin{}), func(msg chan<- *bulletin) {
		for {
			select {
			case <-time.Tick(1 * time.Second):
				msg <- &bulletin{
					Data:        bulletins[rand.Intn(len(bulletins))],
					PublishedAt: time.Now(),
				}
			}
		}
	})

	f.Run()
}
```

## Getting help

- Read [documentation and examples](https://flamego.dev/middleware/sse.html).
- Please [file an issue](https://github.com/flamego/flamego/issues) or [start a discussion](https://github.com/flamego/flamego/discussions) on the [flamego/flamego](https://github.com/flamego/flamego) repository.

## License

This project is under the MIT License. See the [LICENSE](LICENSE) file for the full license text.
