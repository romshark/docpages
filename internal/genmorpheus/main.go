// Command genmorpheus writes the Morpheus bundle of the pages into static/morpheus/ of
// the module: the components of [components] with those they require and the icons
// of [icons]. It takes the version of Morpheus that go.mod requires, which the templ
// components of the pages render and the bundle must match. The bundle has no theme:
// the stylesheet of the pages sets the theme tokens of Morpheus to the colors of
// the site, which the dark mode of theme-default.css would replace.
// Run it in the root of the module, as go generate does.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const module = "github.com/romshark/morpheus"

// components are the Morpheus components that the pages render.
// "morpheus bundle" adds the components they require.
var components = []string{"button", "clipcopy", "dialog", "elastic", "select", "sidebar"}

// icons are the icons of Morpheus that the pages show, by icon in icons.templ,
// and that the components of the pages load, as the caret of <neo-select>.
// The icons that Morpheus lacks are files of static/icons.
var icons = []string{
	"chevron-down", "chevrons-up-down", "file", "github", "menu", "moon", "search", "sun",
}

const dir = "static/morpheus"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "genmorpheus:", err)
		os.Exit(1)
	}
}

func run() error {
	version, err := output("go", "list", "-m", "-f", "{{.Version}}", module)
	if err != nil {
		return fmt.Errorf("version of %s: %w", module, err)
	}
	// A component or icon dropped from the lists leaves no file behind.
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	cmd := exec.Command("go", "run", module+"@"+version, "bundle",
		"-components", strings.Join(components, ","), "-themes", "none", "-o", dir)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	moduleDir, err := output("go", "list", "-m", "-f", "{{.Dir}}", module)
	if err != nil {
		return fmt.Errorf("directory of %s: %w", module, err)
	}
	src := filepath.Join(moduleDir, "internal/site/static/icons")
	if err := os.MkdirAll(filepath.Join(dir, "icons"), 0o755); err != nil {
		return err
	}
	for _, name := range icons {
		svg, err := os.ReadFile(filepath.Join(src, name+".svg"))
		if err != nil {
			return err
		}
		if err := os.WriteFile(
			filepath.Join(dir, "icons", name+".svg"), svg, 0o644,
		); err != nil {
			return err
		}
	}
	return nil
}

// output returns the standard output of the command name with args,
// without the line ending.
func output(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return strings.TrimSpace(string(out)), err
}
