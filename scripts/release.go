//go:build ignore

// Verify downloaded release assets and produce aggregate checksums without publishing.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"tunnelix/internal/release"
)

func main() {
	dir := flag.String("dir", "", "directory containing release manifests and assets")
	version := flag.String("version", "", "expected x.y.z version")
	production := flag.Bool("production", false, "require OS signatures, notarization, and clean sources")
	allowUnsigned := flag.Bool("allow-unsigned", false, "allow missing OS signatures; requires -production and trusted update signatures")
	commit := flag.String("commit", "", "expected source commit")
	platforms := flag.String("platforms", "", "required comma-separated operating systems")
	flag.Parse()
	manifests, e := release.Verify(*dir, *version, os.Getenv("TUNNELIX_UPDATE_PUBLIC_KEY"), release.VerifyOptions{Production: *production, AllowUnsigned: *allowUnsigned})
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	found := map[string]bool{}
	var names []string
	for _, m := range manifests {
		if *commit != "" && m.Commit != *commit {
			fmt.Fprintln(os.Stderr, "unexpected source commit")
			os.Exit(1)
		}
		found[m.Platform] = true
		for _, a := range m.Assets {
			names = append(names, a.Name)
		}
		names = append(names, release.ManifestName(m.Platform, m.Arch))
	}
	if *platforms != "" {
		for _, p := range strings.Split(*platforms, ",") {
			if !found[p] {
				fmt.Fprintln(os.Stderr, "missing platform:", p)
				os.Exit(1)
			}
		}
	}
	if e = release.Checksums(*dir, names, "SHA256SUMS"); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	fmt.Printf("Verified %d platform packages for v%s\n", len(manifests), *version)
}
