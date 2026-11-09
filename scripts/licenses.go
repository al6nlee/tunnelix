//go:build ignore

// Collect redistributed dependency license and notice files without reading application data.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

func main() {
	var out bytes.Buffer
	out.WriteString("Tunnelix dependency licenses\n===========================\n\n")
	add := func(label, root string) {
		entries, e := os.ReadDir(root)
		if e != nil {
			panic(fmt.Errorf("cannot read license directory %q: %w", root, e))
		}
		for _, entry := range entries {
			n := strings.ToLower(entry.Name())
			if entry.IsDir() || !(strings.HasPrefix(n, "license") || strings.HasPrefix(n, "licence") || strings.HasPrefix(n, "notice") || strings.HasPrefix(n, "copying")) {
				continue
			}
			b, e := os.ReadFile(filepath.Join(root, entry.Name()))
			if e != nil {
				panic(e)
			}
			fmt.Fprintf(&out, "\n----- %s / %s -----\n\n%s\n", label, entry.Name(), b)
		}
	}
	add("Go toolchain "+runtime.Version(), runtime.GOROOT())
	// Root requirements cover all supported operating systems without pulling in
	// unrelated tooling dependencies from a framework's transitive module graph.
	b, e := exec.Command("go", "mod", "edit", "-json").Output()
	if e != nil {
		panic(e)
	}
	var gomod struct {
		Require []struct{ Path, Version string }
	}
	if e = json.Unmarshal(b, &gomod); e != nil {
		panic(e)
	}
	sort.Slice(gomod.Require, func(i, j int) bool { return gomod.Require[i].Path < gomod.Require[j].Path })
	for _, requirement := range gomod.Require {
		b, e = exec.Command("go", "mod", "download", "-json", requirement.Path+"@"+requirement.Version).Output()
		if e != nil {
			panic(fmt.Errorf("cannot obtain license source for %s: %w", requirement.Path, e))
		}
		var mod struct{ Dir string }
		if e = json.Unmarshal(b, &mod); e != nil {
			panic(e)
		}
		add(requirement.Path+"@"+requirement.Version, mod.Dir)
	}
	packages := []string{}
	e = filepath.WalkDir("frontend/node_modules", func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() && d.Name() == "package.json" {
			packages = append(packages, filepath.Dir(path))
		}
		return nil
	})
	if e != nil {
		panic(fmt.Errorf("frontend dependencies must be installed before collecting licenses: %w", e))
	}
	sort.Strings(packages)
	for _, path := range packages {
		b, e := os.ReadFile(filepath.Join(path, "package.json"))
		if e != nil {
			panic(e)
		}
		var pkg struct{ Name, Version string }
		if json.Unmarshal(b, &pkg) != nil {
			continue
		}
		add(pkg.Name+"@"+pkg.Version, path)
	}
	if e = os.WriteFile("THIRD_PARTY_LICENSES.txt", out.Bytes(), 0644); e != nil {
		panic(e)
	}
}
