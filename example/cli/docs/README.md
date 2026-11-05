---
name: wc
repository: https://github.com/romshark/docpages
suggested: true
header:
  - title: Docs
    url: README.md
  - title: Options
    url: 2-reference/1-options.md
---

# Introduction

wc counts the lines, words and bytes of files, or of its standard input
without a file.

```sh title="Terminal"
$ printf 'one two\nthree\n' | wc
       2       3      14
```

> [!TIP]
> Check the [installation](1-get-started/1-installation.md), then
> [count the lines](1-get-started/2-count-lines.md) of a project.
