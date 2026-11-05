# Benchmarks

Run the benchmarks of the package with:

```sh
go test -bench . -benchmem
```

| Benchmark | Measures |
| --------- | -------- |
| `BenchmarkPush` | 1 Push into a buffer with room. |
| `BenchmarkPushPop` | 1 Push and 1 Pop on 2 goroutines. |
