# App

[![Ubuntu-latest](https://github.com/igulib/app/actions/workflows/ubuntu-latest.yml/badge.svg)](https://github.com/igulib/app/actions/workflows/ubuntu-latest.yml) [![Macos-latest](https://github.com/igulib/app/actions/workflows/macos-latest.yml/badge.svg)](https://github.com/igulib/app/actions/workflows/macos-latest.yml) [![Windows-latest](https://github.com/igulib/app/actions/workflows/windows-latest.yml/badge.svg)](https://github.com/igulib/app/actions/workflows/windows-latest.yml)
[![Ubuntu-coverage](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/igulib/327daff90ba9c66ebc33ce58f2f98821/raw/app-codecov-ubuntu.json)](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/igulib/327daff90ba9c66ebc33ce58f2f98821/raw/app-codecov-ubuntu.json)

The **app** package simplifies creation and management of
asynchronous Go applications.

It particular, it has means to:
+ create, start, pause, and quit easy-manageable long-living goroutines in
  specified order and with specified timeouts;

+ simplify code by abstracting away the complexity of
  goroutine lifecycle management and concentrate on
  the actual tasks;

+ gracefully handle application shutdown;

+ intercept `SIGINT` and `SIGTERM` signals in order to provide graceful shutdown;

Code example to give you the taste:
```go
// TODO
```

