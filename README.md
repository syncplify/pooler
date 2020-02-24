# Pooler

A minimalistic yet fast worker-pool for Go, with support for custom callback functions.

## Features and philosophy

When we designed `pooler` we had several goals in mind that we wanted to achieve:

- fast worker-pool implementation that only relies on Go channels and atomic
- optional callback functions to receive event notifications from the pool and its goroutines
- optional custom data that can be manipulated from inside the goroutines and/or the callback function
- graceful shutdown of running goroutines

After reviewing several third-party benchmarks, and running a few more of our own, we realized that we wanted to stay away from lists, maps, and mutexes; the key to achieving top speed appeared to be delegating goroutine synchronization entirely to channels, and using `sync/atomic` for counters and to simulate atomic boolean values.

## Istallation

`pooler` is packed as Go module (Go >= 1.11), but it also works just fine when used with older Go versions (<= 1.10). To install it, you may use the typical `go get` command.

```shell
go get -u github.com/syncplify/pooler
```

To learn more, you may also want to [read the documentation](https://pkg.go.dev/github.com/syncplify/pooler).

## Examples

Please take a look at [examples](https://github.com/syncplify/pooler/tree/master/examples) to access a few small example programs that use `pooler`.

## License

This project is licensed under the terms of the Apache 2.0 License. See the [LICENSE](https://github.com/syncplify/pooler/blob/master/LICENSE) file for the full license text.
