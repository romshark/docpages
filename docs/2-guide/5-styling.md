# Styling

Put `style.css` in the docs folder. Every page of every version loads it after the stylesheet of docpages. The site holds it minified, without comments and spaces, and `-serve` serves it as you wrote it. A mistake that a browser would skip, as an unknown property or a `{` without its `}`, fails the build. See [Checks](../3-reference/5-checks.md#stylesheet).

To change the markup of the pages, write a [template](6-templates.md).

## Colors and fonts

docpages draws with CSS custom properties. Override them on `:root`:

```css title="docs/style.css"
:root {
  --bg: light-dark(#ffffff, #0b0b10);
  --text: light-dark(#1b1a24, #e4e2f0);
  --accent: light-dark(#6d3fd6, #a98bff);
  --font-sans: Georgia, serif;
}
```

Write each color as a `light-dark()` pair, the light one first. Then both themes and the theme switch keep working.

| Property | Colors |
| --- | --- |
| `--bg` | The page |
| `--bg-card` | Table headers, code titles, the search button and hovered menu items |
| `--bg-code` | Code |
| `--border` | Borders and lines |
| `--text`, `--text-muted` | Text, and text of less weight |
| `--accent`, `--accent-hover` | Links and highlights |
| `--font-sans`, `--font-mono` | The fonts of text and code |

## Menus and dialogs

The version menu, the theme menu, the search dialog and the navigation are Morpheus components. Your rules don't reach inside them, but these properties do:

| Property | Sets |
| --- | --- |
| `--page-radius` | The corners of buttons, menus and dialogs |
| `--neo-button-shadow`, `--neo-popover-shadow`, `--neo-dialog-shadow` | Their shadows |
| `--neo-duration-scale` | The speed of their animations; `0` turns them off |

## One version

The `<body>` of each page names its version in `data-version`, as `main` or `v1.2`. Use it to style one version:

```css
[data-version="v1.1"] .version-note {
  --note: light-dark(#cf222e, #f85149);
}
```

## Examples

The repository has 2 [examples](../1-get-started/2-examples.md) with styles of their own: a [man page](../1-get-started/2-examples.md#a-command-line-tool) and a [technical one](../1-get-started/2-examples.md#a-go-library). This site has a third, in [docs/style.css](../style.css).
