# App

[![Ubuntu-latest](https://github.com/igulib/app/actions/workflows/ubuntu-latest.yml/badge.svg)](https://github.com/igulib/app/actions/workflows/ubuntu-latest.yml) [![Macos-latest](https://github.com/igulib/app/actions/workflows/macos-latest.yml/badge.svg)](https://github.com/igulib/app/actions/workflows/macos-latest.yml) [![Windows-latest](https://github.com/igulib/app/actions/workflows/windows-latest.yml/badge.svg)](https://github.com/igulib/app/actions/workflows/windows-latest.yml) [![Ubuntu-coverage](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/igulib/3075e755ec4893a57fcaf5b9ecc5dbd2/raw/app-codecov-ubuntu.json)](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/igulib/3075e755ec4893a57fcaf5b9ecc5dbd2/raw/app-codecov-ubuntu.json)

The **app** package simplifies creation and management of
large asynchronous Go applications.

It helps you to:
+ create, start, pause and quit long-living goroutines in
  specified order and with specified timeouts.

+ simplify code by abstracting away the complexity of
  goroutine lifecycle management and concentrate on
  the actual tasks.

+ gracefully handle application shutdown.

+ intercept `SIGINT` and `SIGTERM` signals
  in order to provide graceful shutdown (unix-like systems only).

Basic usage example:
```go

```

