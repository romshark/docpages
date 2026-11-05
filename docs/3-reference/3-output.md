# Output

```
site/
  index.html          redirects to the latest release, or to main
  llms.txt            links the llms.txt of each version
  main/
    index.html        the first page
    index.md          its Markdown copy
    get-started/
      install.html
      install.md
    media/            the images and videos of the pages
    search.json       the search index
    llms.txt          the pages, listed for language models
    llms-full.txt     all pages in 1 Markdown file
    docpages.css
    docpages.js
    morpheus/         the menus and dialogs
    favicon.svg
    style.css
  v1.2/               the same, for each release
```

Every page links its Markdown copy as "Raw Markdown". The links in a copy lead
to the other copies.

`llms.txt` follows the format of [llmstxt.org](https://llmstxt.org).
