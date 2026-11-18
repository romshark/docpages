# Options

```
wc [-c | -m] [-lw] [file...]
```

Without a <ins>file</ins>, or with `-`, wc reads its standard input. In a terminal, <kbd>Ctrl</kbd>+<kbd>D</kbd> ends the input.

## Counts

| Option | Counts |
| ------ | ------ |
| `-c` | Bytes. |
| `-l` | Newline characters. |
| `-m` | Characters, which differ from bytes in UTF-8.<br>`é` is 1 character and 2 bytes. |
| `-w` | Words, the runs of characters between white space. |

Without an option, wc counts lines, words and bytes, as `-lwc`.

## Output

wc writes 1 line per file: the counts in the order lines, words, bytes or characters, whatever the order of the options, then the name of the file. The width of the columns differs between implementations.

## Exit status

wc exits with 0 when it read every file and with more than 0 otherwise.
