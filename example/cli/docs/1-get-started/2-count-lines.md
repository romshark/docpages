# Count lines

[`-l`](../2-reference/1-options.md#counts) counts lines alone. With more than
1 file, wc adds a total:

```sh
$ wc -l main.go util.go
     120 main.go
      45 util.go
     165 total
```

## Count the files of a project

Git lists the files that it tracks, and `xargs` passes them to wc:

```sh
git ls-files | xargs wc -l
```

> [!WARNING]
> wc counts newline characters, so it misses a last line without one:
> `printf 'a\nb' | wc -l` prints 1.
