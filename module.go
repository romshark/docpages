package docpages

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// commandPath is the package path of the docpages command.
const commandPath = modulePath + "/cmd/docpages"

// docsModule is the Go module of a docs directory with .templ files.
// Its go.mod requires docpages, and its root package is the package of the .templ files.
type docsModule struct {
	file  string // Path of go.mod from the repository root, as "docs/go.mod".
	path  string // Module path.
	goMod []byte
	goSum []byte // Empty without go.sum.
	// docpages is the version of docpages that go.mod requires.
	docpages string
	// local reports whether go.mod replaces docpages with a directory, as the
	// docs of docpages do. Its requirement then names no release.
	local bool
	// goFiles are the module's .go files from the repository root.
	goFiles []string
}

// readModule returns the module of docs directory dir in fsys.
func readModule(fsys fs.FS, dir string) (*docsModule, error) {
	file := path.Join(dir, "go.mod")
	data, err := fs.ReadFile(fsys, file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s: the .templ files need a Go module "+
			"that requires %s\nrun: cd %s && go mod init docs && %s",
			dir, modulePath, dir, getTool())
	}
	if err != nil {
		return nil, err
	}
	f, err := modfile.Parse(file, data, nil)
	if err != nil {
		return nil, err
	}
	if f.Module == nil {
		return nil, fmt.Errorf("%s: no module directive", file)
	}
	m := &docsModule{file: file, path: f.Module.Mod.Path, goMod: data}
	for _, r := range f.Require {
		if r.Mod.Path == modulePath {
			m.docpages = r.Mod.Version
		}
	}
	if m.docpages == "" {
		return nil, fmt.Errorf("%s: no requirement of %s\nrun: cd %s && %s",
			file, modulePath, dir, getTool())
	}
	for _, r := range f.Replace {
		if r.Old.Path == modulePath && modfile.IsDirectoryPath(r.New.Path) {
			m.local = true
		}
	}
	m.goSum, err = fs.ReadFile(fsys, path.Join(dir, "go.sum"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	err = fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			// The go command skips these directories, and a go.mod starts another module.
			if p != dir && (strings.HasPrefix(name, ".") ||
				strings.HasPrefix(name, "_") || name == "testdata") {
				return fs.SkipDir
			}
			if _, err := fs.Stat(fsys, path.Join(p, "go.mod")); p != dir && err == nil {
				return fs.SkipDir
			}
			return nil
		}
		if path.Ext(name) == ".go" {
			m.goFiles = append(m.goFiles, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}

// files returns m's go.mod, go.sum and .go files from the repository root.
func (m *docsModule) files() []string {
	r := append([]string{m.file}, m.goFiles...)
	if len(m.goSum) > 0 {
		r = append(r, path.Join(path.Dir(m.file), "go.sum"))
	}
	return r
}

// getTool returns the command that makes a module require the running docpages
// as a tool. The tool directive keeps the requirement through go mod tidy,
// which sees no import of docpages in .templ files, and lets go tool docpages
// run the required version.
func getTool() string {
	return "go get -tool " + commandPath + "@" + cmp.Or(releaseVersion(), "latest")
}

// runningVersion returns the version of the running docpages, as
// [buildVersion] does. Tests replace it.
var runningVersion = buildVersion

// releaseVersion returns the running docpages release, as "v1.2.3",
// or "" for a development build or a pseudo-version.
func releaseVersion() string {
	v := runningVersion()
	if !semver.IsValid(v) || module.IsPseudoVersion(v) || semver.Build(v) != "" {
		return ""
	}
	return v
}

// compatible reports whether docpages release running can build the templates
// of a module that requires docpages version required. Following SemVer,
// it must have the same major version and be no older. Below v1,
// where SemVer promises nothing, the minor version acts as the major one.
func compatible(running, required string) bool {
	if semver.Compare(running, required) < 0 {
		return false
	}
	if semver.Major(running) == "v0" {
		return semver.MajorMinor(running) == semver.MajorMinor(required)
	}
	return semver.Major(running) == semver.Major(required)
}

// VersionError reports a docs module that requires a version of docpages that
// the running docpages isn't compatible with.
type VersionError struct {
	GoMod    string // Path of go.mod from the repository root, as "docs/go.mod".
	Required string // Version of docpages that go.mod requires, as "v1.3.0".
	Running  string // Version of the running docpages, as "v1.2.0".
}

func (e *VersionError) Error() string {
	return fmt.Sprintf("%s requires %s %s, incompatible with the running %s",
		e.GoMod, modulePath, e.Required, e.Running)
}

// checkVersion returns a [*VersionError] if the running docpages is a release
// that isn't compatible with the version of docpages that m requires.
// Development builds, pseudo-versions and a docpages that m replaces with a
// directory pass.
func checkVersion(m *docsModule) error {
	running := releaseVersion()
	if running == "" || m.local || compatible(running, m.docpages) {
		return nil
	}
	return &VersionError{GoMod: m.file, Required: m.docpages, Running: running}
}
