package release

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path"
	"strings"
)

func archivePath(name, root string) bool {
	clean := path.Clean(name)
	return clean == strings.TrimSuffix(name, "/") && !strings.Contains(name, "\\") && !strings.HasPrefix(name, "/") && (clean == root || strings.HasPrefix(clean, root+"/"))
}
func verifyZIP(filename, root string, required []string) error {
	r, e := zip.OpenReader(filename)
	if e != nil {
		return e
	}
	defer r.Close()
	seen := map[string]bool{}
	var total uint64
	for _, f := range r.File {
		if !archivePath(f.Name, root) || f.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe or unexpected ZIP layout")
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if seen[f.Name] {
			return errors.New("duplicate ZIP entry")
		}
		seen[f.Name] = true
		total += f.UncompressedSize64
		if total > 512<<20 {
			return errors.New("archive expands beyond 512 MiB")
		}
	}
	for _, name := range required {
		if !seen[name] {
			return errors.New("required ZIP entry missing: " + name)
		}
	}
	return nil
}
func verifyTar(filename string) error {
	f, e := os.Open(filename)
	if e != nil {
		return e
	}
	defer f.Close()
	g, e := gzip.NewReader(f)
	if e != nil {
		return e
	}
	defer g.Close()
	r := tar.NewReader(g)
	seen := map[string]bool{}
	var total int64
	for {
		h, e := r.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		if !archivePath(h.Name, "Tunnelix") {
			return errors.New("unsafe tar path")
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if h.Typeflag != tar.TypeReg {
			return errors.New("unsupported tar entry")
		}
		if seen[h.Name] {
			return errors.New("duplicate tar entry")
		}
		seen[h.Name] = true
		total += h.Size
		if total > 512<<20 {
			return errors.New("archive expands beyond 512 MiB")
		}
		if _, e = io.Copy(io.Discard, r); e != nil {
			return e
		}
	}
	for _, name := range []string{"Tunnelix/tunnelix", "Tunnelix/LICENSE", "Tunnelix/THIRD_PARTY_LICENSES.txt", "Tunnelix/THIRD_PARTY_NOTICES.md"} {
		if !seen[name] {
			return errors.New("required tar entry missing: " + name)
		}
	}
	return nil
}
