// Package release describes and verifies the files handed to users and the updater.
package release

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"tunnelix/internal/updates"
)

type Asset struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type Manifest struct {
	Schema          int     `json:"schema"`
	Version         string  `json:"version"`
	Platform        string  `json:"platform"`
	Arch            string  `json:"arch"`
	Commit          string  `json:"commit"`
	Dirty           bool    `json:"dirty"`
	OSSigned        bool    `json:"osSigned"`
	Notarized       bool    `json:"notarized"`
	UpdatePublicKey string  `json:"updatePublicKey"`
	Assets          []Asset `json:"assets"`
}

func ValidTarget(platform, arch string) bool {
	return (platform == "darwin" || platform == "linux") && (arch == "amd64" || arch == "arm64") || platform == "windows" && arch == "amd64"
}
func BaseName(version, platform, arch string) string {
	return "Tunnelix-" + version + "-" + platform + "-" + arch
}
func ManifestName(platform, arch string) string { return "release-" + platform + "-" + arch + ".json" }
func Inspect(path string) (Asset, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Asset{}, err
	}
	if !info.Mode().IsRegular() {
		return Asset{}, fmt.Errorf("not a regular file: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return Asset{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return Asset{}, err
	}
	return Asset{Name: filepath.Base(path), Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}
func Write(dir string, m Manifest, names []string) error {
	m.Schema = 1
	m.Assets = nil
	for _, name := range names {
		if !safeName(name) {
			return errors.New("invalid asset name")
		}
		a, e := Inspect(filepath.Join(dir, name))
		if e != nil {
			return e
		}
		m.Assets = append(m.Assets, a)
	}
	sort.Slice(m.Assets, func(i, j int) bool { return m.Assets[i].Name < m.Assets[j].Name })
	data, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return e
	}
	name := ManifestName(m.Platform, m.Arch)
	if e = os.WriteFile(filepath.Join(dir, name), append(data, '\n'), 0644); e != nil {
		return e
	}
	return Checksums(dir, append(append([]string{}, names...), name), "SHA256SUMS-"+m.Platform+"-"+m.Arch)
}
func safeName(name string) bool {
	return name != "" && name != "." && filepath.Base(name) == name && !strings.ContainsAny(name, "\\/\r\n\t ")
}
func Checksums(dir string, names []string, out string) error {
	sort.Strings(names)
	var text strings.Builder
	for _, name := range names {
		if !safeName(name) {
			return errors.New("invalid asset name")
		}
		a, e := Inspect(filepath.Join(dir, name))
		if e != nil {
			return e
		}
		fmt.Fprintf(&text, "%s  %s\n", a.SHA256, a.Name)
	}
	return os.WriteFile(filepath.Join(dir, out), []byte(text.String()), 0644)
}

type VerifyOptions struct {
	Production    bool
	AllowUnsigned bool // Permit missing OS signatures only; update signatures remain required.
}

// Verify checks actual bytes, signed update manifests, required assets, and release policy.
// expectedKey is supplied independently of the release metadata; it is the trust anchor.
func Verify(dir, version, expectedKey string, options VerifyOptions) ([]Manifest, error) {
	if options.AllowUnsigned && !options.Production {
		return nil, errors.New("allow-unsigned requires production verification")
	}
	if !updates.ValidVersion(version) {
		return nil, errors.New("version must be x.y.z")
	}
	paths, e := filepath.Glob(filepath.Join(dir, "release-*.json"))
	if e != nil || len(paths) == 0 {
		return nil, errors.New("no release manifests")
	}
	var result []Manifest
	seen := map[string]bool{}
	expectedFiles := map[string]bool{"SHA256SUMS": true}
	sourceCommit := ""
	for _, path := range paths {
		info, e := os.Lstat(path)
		if e != nil {
			return nil, e
		}
		if !info.Mode().IsRegular() || info.Size() > 64<<10 {
			return nil, errors.New("invalid release manifest file")
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return nil, e
		}
		var m Manifest
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if e = dec.Decode(&m); e != nil {
			return nil, e
		}
		var trailing any
		if e = dec.Decode(&trailing); e != io.EOF {
			return nil, errors.New("trailing manifest data")
		}
		if m.Schema != 1 || m.Version != version || !ValidTarget(m.Platform, m.Arch) || filepath.Base(path) != ManifestName(m.Platform, m.Arch) {
			return nil, errors.New("invalid release identity")
		}
		target := m.Platform + "-" + m.Arch
		if seen[target] {
			return nil, errors.New("duplicate target")
		}
		seen[target] = true
		if options.Production {
			if sourceCommit != "" && sourceCommit != m.Commit {
				return nil, errors.New("release targets have different source commits")
			}
			sourceCommit = m.Commit
			commit, e := hex.DecodeString(m.Commit)
			if e != nil || len(commit) != 20 || m.Dirty {
				return nil, errors.New("production release requires a clean Git commit")
			}
			if !options.AllowUnsigned && m.Platform != "linux" && !m.OSSigned {
				return nil, errors.New("production release requires OS code signing")
			}
			if !options.AllowUnsigned && m.Platform == "darwin" && !m.Notarized {
				return nil, errors.New("production macOS release requires notarization")
			}
			if expectedKey == "" {
				return nil, errors.New("production release requires a trusted update public key")
			}
		}
		base := BaseName(version, m.Platform, m.Arch)
		portable := base + ".zip"
		if m.Platform == "linux" {
			portable = base + ".tar.gz"
		}
		update := updates.ArtifactName(m.Platform, m.Arch)
		allowed := map[string]bool{portable: true, update: true, update + ".update.json": true}
		installer := ""
		if m.Platform == "darwin" {
			installer = base + ".dmg"
		}
		if m.Platform == "windows" {
			installer = base + "-setup.exe"
		}
		if installer != "" {
			allowed[installer] = true
		}
		files := map[string]bool{}
		for _, a := range m.Assets {
			if !safeName(a.Name) || files[a.Name] || !allowed[a.Name] || a.Size <= 0 {
				return nil, errors.New("unexpected or duplicate asset")
			}
			files[a.Name] = true
			expectedFiles[a.Name] = true
			actual, e := Inspect(filepath.Join(dir, a.Name))
			if e != nil {
				return nil, e
			}
			if actual != a {
				return nil, fmt.Errorf("asset checksum mismatch: %s", a.Name)
			}
		}
		if !files[portable] || !files[update] || options.Production && installer != "" && !files[installer] {
			return nil, errors.New("missing distribution artifact")
		}
		if m.UpdatePublicKey != "" || expectedKey != "" || files[update+".update.json"] {
			if expectedKey == "" || m.UpdatePublicKey != expectedKey {
				return nil, errors.New("update key does not match trusted public key")
			}
			if e = verifyUpdate(dir, version, update, expectedKey); e != nil {
				return nil, e
			}
			if !files[update+".update.json"] {
				return nil, errors.New("signed manifest missing from asset list")
			}
		}
		if m.Platform == "linux" {
			if e = verifyTar(filepath.Join(dir, portable)); e != nil {
				return nil, e
			}
		} else {
			root := "Tunnelix"
			required := []string{"Tunnelix/tunnelix.exe", "Tunnelix/LICENSE", "Tunnelix/THIRD_PARTY_LICENSES.txt", "Tunnelix/THIRD_PARTY_NOTICES.md"}
			if m.Platform == "darwin" {
				root = "Tunnelix.app"
				required = []string{root + "/Contents/MacOS/Tunnelix", root + "/Contents/Info.plist", root + "/Contents/Resources/LICENSE", root + "/Contents/Resources/THIRD_PARTY_LICENSES.txt", root + "/Contents/Resources/THIRD_PARTY_NOTICES.md"}
			}
			if e = verifyZIP(filepath.Join(dir, portable), root, required); e != nil {
				return nil, e
			}
		}
		updateRoot := "tunnelix"
		requiredUpdate := []string{updateRoot}
		if m.Platform == "windows" {
			updateRoot = "tunnelix.exe"
			requiredUpdate = []string{updateRoot}
		}
		if m.Platform == "darwin" {
			updateRoot = "Tunnelix.app"
			requiredUpdate = []string{updateRoot + "/Contents/MacOS/Tunnelix", updateRoot + "/Contents/Info.plist", updateRoot + "/Contents/Resources/THIRD_PARTY_LICENSES.txt"}
		}
		if e = verifyZIP(filepath.Join(dir, update), updateRoot, requiredUpdate); e != nil {
			return nil, e
		}
		manifestName := ManifestName(m.Platform, m.Arch)
		expectedFiles[manifestName] = true
		sumName := "SHA256SUMS-" + m.Platform + "-" + m.Arch
		expectedFiles[sumName] = true
		names := []string{manifestName}
		for _, a := range m.Assets {
			names = append(names, a.Name)
		}
		sort.Strings(names)
		var want strings.Builder
		for _, name := range names {
			a, e := Inspect(filepath.Join(dir, name))
			if e != nil {
				return nil, e
			}
			fmt.Fprintf(&want, "%s  %s\n", a.SHA256, a.Name)
		}
		actual, e := os.ReadFile(filepath.Join(dir, sumName))
		if e != nil {
			return nil, e
		}
		if string(actual) != want.String() {
			return nil, errors.New("invalid platform checksums")
		}
		result = append(result, m)
	}
	entries, e := os.ReadDir(dir)
	if e != nil {
		return nil, e
	}
	for _, entry := range entries {
		if !expectedFiles[entry.Name()] || !entry.Type().IsRegular() {
			return nil, fmt.Errorf("unexpected release file: %s", entry.Name())
		}
	}
	return result, nil
}
func verifyUpdate(dir, version, name, key string) error {
	pub, e := base64.StdEncoding.DecodeString(key)
	if e != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("invalid public key")
	}
	raw, e := os.ReadFile(filepath.Join(dir, name+".update.json"))
	if e != nil {
		return e
	}
	var env updates.Envelope
	if json.Unmarshal(raw, &env) != nil || !ed25519.Verify(pub, env.Payload, env.Signature) {
		return errors.New("invalid manifest signature")
	}
	var m updates.Manifest
	if json.Unmarshal(env.Payload, &m) != nil || m.Version != version || m.Filename != name || m.Size > updates.MaxArtifactSize {
		return errors.New("invalid signed update identity")
	}
	a, e := Inspect(filepath.Join(dir, name))
	if e != nil {
		return e
	}
	digest, e := hex.DecodeString(a.SHA256)
	if e != nil || m.Size != a.Size || m.SHA256 != a.SHA256 || !ed25519.Verify(pub, digest, m.ArtifactSignature) {
		return errors.New("invalid update artifact signature")
	}
	return nil
}
