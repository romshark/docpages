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
    sw.js             the service worker of offline viewing
    offline.json      the files that offline viewing saves
  v1.2/               the same, for each release
```

Every page links its Markdown copy as "Raw Markdown". The links in a copy lead to the other copies.

`llms.txt` follows the format of [llmstxt.org](https://llmstxt.org).

## Offline viewing

The header of every page has a button that saves the version for offline viewing. Its checkboxes of pages, images, and videos and audio, each shown with its size, are checked for the groups that the browser holds. Apply downloads the groups that the reader checked, with a progress bar, and then deletes the ones that the reader unchecked, after a dialog that names them and their size. Cancel stops the download and keeps the groups that the browser held before. A group that doesn't fit into the free storage of the browser can't be checked, and its info button says so. Offline, an image or a video that the reader didn't save shows "Not available offline" in its place.

`sw.js`, the service worker of the version, keeps the files in the cache of the browser and serves them when the network fails. The browser registers it only when a reader saves the version, and drops it when the reader removes the saved files, so a reader who never saves loads the site without it. The page fetches `offline.json` after it has loaded. `offline.json` lists the files of each group. Both hold an ID of the build, so a new build, as a new patch tag or a new docpages, makes the browser drop the saved files and download the same groups again. A page that loads online checks for a new `sw.js`, and a new worker takes over at once.

- A download that a closed tab stopped goes on when the reader opens a page of the version again.
- A service worker needs HTTPS, or `localhost`. GitHub Pages serves HTTPS.
- `-offline=false` builds the site without offline viewing. A reader who saved a version of an earlier build loses the saved files on the next visit.
- All GitHub Pages sites of an account share 1 origin, and so 1 share of the storage of the browser.
- Safari deletes the saved files after 7 days of use without a visit to the site, unless the reader added it to the Home Screen.
- Pages load Mermaid from a CDN. A saved version keeps the files of Mermaid that its pages loaded online.
