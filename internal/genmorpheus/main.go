// Command genmorpheus writes the page bundle to static/morpheus/: [components]
// and their dependencies plus [icons]. It uses go.mod's Morpheus version so the
// bundle matches rendered templ components. The bundle omits a theme because
// page CSS sets Morpheus tokens to site colors, which theme-default.css dark mode
// would override. Run it at the module root, as go generate does.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const module = "github.com/romshark/morpheus"

// components are page-rendered Morpheus components; "morpheus bundle" adds dependencies.
var components = []string{
	"aura", "button", "checkbox", "clipcopy", "dialog", "elastic", "popover", "progress",
	"select", "sidebar", "tooltip",
}

// icons are Morpheus icons shown by icons.templ or loaded by page components,
// such as <neo-select>'s caret. Missing Morpheus icons live in static/icons.
var icons = []string{
	"chevron-down", "chevrons-up-down", "circle-check", "download", "file", "github",
	"info", "menu", "moon", "search", "sun",
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
	// Removing a listed component or icon leaves no stale file.
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

// output returns command name's standard output without its line ending.
func output(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return strings.TrimSpace(string(out)), err
}
