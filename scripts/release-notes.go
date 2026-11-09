//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"tunnelix/internal/updates"
)

func main() {
	version := flag.String("version", "", "x.y.z version")
	out := flag.String("out", "", "destination Markdown file")
	flag.Parse()
	if !updates.ValidVersion(*version) {
		panic("version must be x.y.z")
	}
	b, e := os.ReadFile("CHANGELOG.md")
	if e != nil {
		panic(e)
	}
	heading := "## [" + *version + "]"
	found := false
	var notes []string
	for _, line := range strings.Split(string(b), "\n") {
		if line == heading || strings.HasPrefix(line, heading+" ") {
			found = true
			continue
		}
		if found && strings.HasPrefix(line, "## ") {
			break
		}
		if found {
			notes = append(notes, line)
		}
	}
	body := strings.TrimSpace(strings.Join(notes, "\n"))
	if !found || body == "" {
		panic("version has no changelog entry")
	}
	body += "\n\nSee SHA256SUMS for artifact checksums. / 安装前可使用 SHA256SUMS 校验下载文件。\n"
	if *out == "" {
		fmt.Print(body)
	} else if e = os.WriteFile(*out, []byte(body), 0644); e != nil {
		panic(e)
	}
}
