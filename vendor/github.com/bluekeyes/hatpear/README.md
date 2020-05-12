# hatpear [![GoDoc](https://godoc.org/github.com/bluekeyes/hatpear?status.svg)](http://godoc.org/github.com/bluekeyes/hatpear)

hatpear (from "httperr") is a simple, unopinionated Go 1.7+ library for
capturing and responding to errors that occur while handling HTTP requests. It
has no dependencies and works well with the standard library or other HTTP
libraries that use standard types.

See the [package documentation](https://godoc.org/github.com/bluekeyes/hatpear) for
examples and usage details.

**Stability Note:** While the API is simple, it hasn't seen heavy use yet and
may change in the future. I recommend vendoring this package at a specific
commit if you are concerned about API changes.
