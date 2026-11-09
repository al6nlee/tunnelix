package release

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"tunnelix/internal/updates"
)

func zipData(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, data := range entries {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0755)
		w, e := z.CreateHeader(h)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte(data)); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}

type fixture struct {
	dir, key string
	private  ed25519.PrivateKey
	manifest Manifest
	names    []string
}

func newFixture(t *testing.T, platform string) *fixture {
	t.Helper()
	pub, priv, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	f := &fixture{dir: t.TempDir(), key: base64.StdEncoding.EncodeToString(pub), private: priv}
	f.manifest = Manifest{Version: "1.2.3", Platform: platform, Arch: "amd64", Commit: strings.Repeat("a", 40), OSSigned: true, Notarized: true, UpdatePublicKey: f.key}
	base := BaseName("1.2.3", platform, "amd64")
	portable := base + ".zip"
	entries := map[string]string{}
	updateEntries := map[string]string{}
	if platform == "darwin" {
		for _, name := range []string{"Contents/MacOS/Tunnelix", "Contents/Info.plist", "Contents/Resources/LICENSE", "Contents/Resources/THIRD_PARTY_LICENSES.txt", "Contents/Resources/THIRD_PARTY_NOTICES.md"} {
			entries["Tunnelix.app/"+name] = "fixture"
		}
		updateEntries = entries
	} else {
		exe := "tunnelix"
		if platform == "windows" {
			exe += ".exe"
		}
		for _, name := range []string{exe, "LICENSE", "THIRD_PARTY_LICENSES.txt", "THIRD_PARTY_NOTICES.md"} {
			entries["Tunnelix/"+name] = "fixture"
		}
		updateEntries[exe] = "fixture"
	}
	write := func(name string, data []byte) {
		t.Helper()
		if e := os.WriteFile(filepath.Join(f.dir, name), data, 0644); e != nil {
			t.Fatal(e)
		}
		f.names = append(f.names, name)
	}
	if platform == "linux" {
		portable = base + ".tar.gz"
		var b bytes.Buffer
		g := gzip.NewWriter(&b)
		w := tar.NewWriter(g)
		for name, data := range entries {
			if e = w.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(data))}); e != nil {
				t.Fatal(e)
			}
			w.Write([]byte(data))
		}
		w.Close()
		g.Close()
		write(portable, b.Bytes())
	} else {
		write(portable, zipData(t, entries))
		if platform == "darwin" {
			write(base+".dmg", []byte("fixture installer"))
		} else {
			write(base+"-setup.exe", []byte("fixture installer"))
		}
	}
	name := updates.ArtifactName(platform, "amd64")
	data := zipData(t, updateEntries)
	write(name, data)
	signed, e := updates.SignedManifest("1.2.3", name, data, priv)
	if e != nil {
		t.Fatal(e)
	}
	write(name+".update.json", signed)
	if e = Write(f.dir, f.manifest, f.names); e != nil {
		t.Fatal(e)
	}
	return f
}
func TestReleaseVerification(t *testing.T) {
	for _, platform := range []string{"darwin", "windows", "linux"} {
		t.Run(platform, func(t *testing.T) {
			f := newFixture(t, platform)
			m, e := Verify(f.dir, "1.2.3", f.key, VerifyOptions{Production: true})
			if e != nil || len(m) != 1 {
				t.Fatalf("verify: %v", e)
			}
		})
	}
}
func TestReleaseRejectsBadInputs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*testing.T, *fixture)
	}{
		{"corrupt artifact", func(t *testing.T, f *fixture) {
			os.WriteFile(filepath.Join(f.dir, f.names[0]), []byte("corrupt"), 0644)
		}},
		{"missing artifact", func(t *testing.T, f *fixture) { os.Remove(filepath.Join(f.dir, f.names[0])) }},
		{"extra file", func(t *testing.T, f *fixture) {
			os.WriteFile(filepath.Join(f.dir, "unexpected.txt"), []byte("extra"), 0644)
		}},
		{"bad checksums", func(t *testing.T, f *fixture) {
			os.WriteFile(filepath.Join(f.dir, "SHA256SUMS-windows-amd64"), []byte("bad"), 0644)
		}},
		{"wrong public key", func(t *testing.T, f *fixture) {
			pub, _, _ := ed25519.GenerateKey(rand.Reader)
			f.key = base64.StdEncoding.EncodeToString(pub)
		}},
		{"changed source", func(t *testing.T, f *fixture) { f.manifest.Dirty = true; Write(f.dir, f.manifest, f.names) }},
		{"missing OS signature", func(t *testing.T, f *fixture) { f.manifest.OSSigned = false; Write(f.dir, f.manifest, f.names) }},
		{"wrong version", func(t *testing.T, f *fixture) { f.manifest.Version = "1.2.4"; Write(f.dir, f.manifest, f.names) }},
		{"forged checksum", func(t *testing.T, f *fixture) {
			name := updates.ArtifactName("windows", "amd64")
			os.WriteFile(filepath.Join(f.dir, name), zipData(t, map[string]string{"tunnelix.exe": "changed"}), 0644)
			Write(f.dir, f.manifest, f.names)
		}},
		{"unsafe signed layout", func(t *testing.T, f *fixture) {
			name := updates.ArtifactName("windows", "amd64")
			data := zipData(t, map[string]string{"tunnelix.exe": "fixture", "../outside": "bad"})
			os.WriteFile(filepath.Join(f.dir, name), data, 0644)
			signed, _ := updates.SignedManifest("1.2.3", name, data, f.private)
			os.WriteFile(filepath.Join(f.dir, name+".update.json"), signed, 0644)
			Write(f.dir, f.manifest, f.names)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, "windows")
			tc.change(t, f)
			if _, e := Verify(f.dir, "1.2.3", f.key, VerifyOptions{Production: true}); e == nil {
				t.Fatal("invalid release accepted")
			}
		})
	}
}
func TestMacRequiresNotarization(t *testing.T) {
	f := newFixture(t, "darwin")
	f.manifest.Notarized = false
	Write(f.dir, f.manifest, f.names)
	if _, e := Verify(f.dir, "1.2.3", f.key, VerifyOptions{Production: true}); e == nil {
		t.Fatal("unnotarized production release accepted")
	}
	if _, e := Verify(f.dir, "1.2.3", f.key, VerifyOptions{}); e != nil {
		t.Fatal(e)
	}
}
func TestNoTrustInMetadataKeyAlone(t *testing.T) {
	f := newFixture(t, "linux")
	if _, e := Verify(f.dir, "1.2.3", "", VerifyOptions{}); e == nil {
		t.Fatal("metadata key used as trust anchor")
	}
}

func TestUnsignedPreviewVerification(t *testing.T) {
	options := VerifyOptions{Production: true, AllowUnsigned: true}
	for _, platform := range []string{"darwin", "windows", "linux"} {
		t.Run(platform, func(t *testing.T) {
			f := newFixture(t, platform)
			f.manifest.OSSigned = false
			f.manifest.Notarized = false
			if err := Write(f.dir, f.manifest, f.names); err != nil {
				t.Fatal(err)
			}
			if _, err := Verify(f.dir, "1.2.3", f.key, options); err != nil {
				t.Fatal(err)
			}
			if platform != "linux" {
				if _, err := Verify(f.dir, "1.2.3", f.key, VerifyOptions{Production: true}); err == nil {
					t.Fatal("strict production accepted an unsigned preview")
				}
			}
		})
	}
	f := newFixture(t, "darwin")
	f.manifest.Notarized = false
	if err := Write(f.dir, f.manifest, f.names); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(f.dir, "1.2.3", f.key, options); err != nil {
		t.Fatal("preview should allow missing notarization", err)
	}
	if _, err := Verify(f.dir, "1.2.3", f.key, VerifyOptions{AllowUnsigned: true}); err == nil {
		t.Fatal("unsigned policy must not select development verification")
	}
}

func TestUnsignedPreviewStillRequiresTrustedArtifacts(t *testing.T) {
	for _, name := range []string{"dirty source", "missing commit", "missing public key", "wrong public key", "missing installer", "missing update signature", "forged update"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, "windows")
			f.manifest.OSSigned = false
			update := updates.ArtifactName("windows", "amd64")
			switch name {
			case "dirty source":
				f.manifest.Dirty = true
			case "missing commit":
				f.manifest.Commit = ""
			case "missing public key":
				f.key = ""
			case "wrong public key":
				public, _, err := ed25519.GenerateKey(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				f.key = base64.StdEncoding.EncodeToString(public)
			case "missing installer", "missing update signature":
				missing := BaseName("1.2.3", "windows", "amd64") + "-setup.exe"
				if name == "missing update signature" {
					missing = update + ".update.json"
				}
				if err := os.Remove(filepath.Join(f.dir, missing)); err != nil {
					t.Fatal(err)
				}
				var names []string
				for _, asset := range f.names {
					if asset != missing {
						names = append(names, asset)
					}
				}
				f.names = names
			case "forged update":
				if err := os.WriteFile(filepath.Join(f.dir, update), zipData(t, map[string]string{"tunnelix.exe": "tampered"}), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := Write(f.dir, f.manifest, f.names); err != nil {
				t.Fatal(err)
			}
			if _, err := Verify(f.dir, "1.2.3", f.key, VerifyOptions{Production: true, AllowUnsigned: true}); err == nil {
				t.Fatal("unsigned preview accepted untrusted or incomplete artifacts")
			}
		})
	}
}
func TestCanonicalArchivePaths(t *testing.T) {
	for _, name := range []string{"../Tunnelix/a", "Tunnelix/a/../b", "/Tunnelix/a", "Tunnelix\\a", "Tunnelix//a"} {
		if archivePath(name, "Tunnelix") {
			t.Fatal(name)
		}
	}
}
