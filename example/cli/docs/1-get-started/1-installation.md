---
suggested: true
---

# Installation

Every Unix-like system ships wc: GNU coreutils on most Linux distributions, BusyBox on Alpine and the BSD wc on macOS and the BSDs. Git for Windows brings it to Git Bash.

Check that the shell finds it:

```sh
command -v wc
```

> [!NOTE]
> The implementations agree on the options of [POSIX](../2-reference/1-options.md) and differ in the others. `man wc` lists those of yours.
