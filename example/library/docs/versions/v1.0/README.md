# Overview

ringbuf is a fixed-size ring buffer for Go, for 1 goroutine that pushes and 1
that pops, without locks.

```go title="main.go"
b := ringbuf.New[int](1024)
b.Push(42)
v, ok := b.Pop() // 42, true
```

[Concurrency](1-guide/1-concurrency.md) explains what the buffer guarantees.
