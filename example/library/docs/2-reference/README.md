---
title: API
---

# API reference

```go
import "example.com/ringbuf"
```

## New

```go
func New[T any](size int) *Buffer[T]
```

New returns an empty buffer for `size` values. `size` must be a power of 2.

<details>
<summary>Why a power of 2?</summary>

The buffer wraps its indexes with a mask, `i & (size - 1)`, which costs less than `i % size`. The mask wraps an index only for a power of 2.

</details>

## Push

```go
func (b *Buffer[T]) Push(v T) bool
```

Push appends `v` and reports whether the buffer had room for it.

## Pop

```go
func (b *Buffer[T]) Pop() (T, bool)
```

Pop removes the oldest value and reports whether there was one.
