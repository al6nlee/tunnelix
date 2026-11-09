package updates

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fixture struct {
	provider           *Provider
	manifest, artifact []byte
	tag                string
	code               int
	filename           string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: "tunnelix", Method: zip.Deflate}
	h.SetMode(0755)
	w, err := z.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("isolated test executable"))
	z.Close()
	f := &fixture{artifact: buf.Bytes(), tag: "v1.1.0", code: 200, filename: ArtifactName("linux", "amd64")}
	f.manifest, err = SignedManifest("1.1.0", f.filename, f.artifact, priv)
	if err != nil {
		t.Fatal(err)
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/al6nlee/tunnelix/releases/latest":
			w.WriteHeader(f.code)
			json.NewEncoder(w).Encode(map[string]any{"tag_name": f.tag, "body": "Test release", "assets": []map[string]string{{"name": f.filename, "browser_download_url": server.URL + "/artifact"}, {"name": f.filename + ".update.json", "browser_download_url": server.URL + "/manifest"}}})
		case "/manifest":
			w.Write(f.manifest)
		case "/artifact":
			w.Write(f.artifact)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	f.provider = &Provider{Client: server.Client(), BaseURL: server.URL, PublicKey: pub}
	return f
}
func request() updater.CheckRequest {
	return updater.CheckRequest{CurrentVersion: "1.0.0", Platform: "linux", Arch: "amd64"}
}
func TestSignedFeed(t *testing.T) {
	f := newFixture(t)
	r, e := f.provider.Check(context.Background(), request())
	if e != nil || r == nil || r.Version != "1.1.0" {
		t.Fatalf("release %v: %v", r, e)
	}
	var out bytes.Buffer
	if e = f.provider.Download(context.Background(), r, &out, nil); e != nil || !bytes.Equal(out.Bytes(), f.artifact) {
		t.Fatalf("download: %v", e)
	}
}
func TestFeedRejectsInvalidReleases(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		mutate     func(*fixture)
	}{
		{"missing", "UPDATE_NO_RELEASE", func(f *fixture) { f.code = 404 }},
		{"server", "UPDATE_CHECK_FAILED", func(f *fixture) { f.code = 503 }},
		{"unsigned", "UPDATE_VERIFY_FAILED", func(f *fixture) { f.manifest = []byte(`{"payload":"e30="}`) }},
		{"tampered", "UPDATE_VERIFY_FAILED", func(f *fixture) {
			var v Envelope
			json.Unmarshal(f.manifest, &v)
			v.Payload[0] ^= 1
			f.manifest, _ = json.Marshal(v)
		}},
		{"mismatched version", "UPDATE_VERIFY_FAILED", func(f *fixture) { f.tag = "v1.2.0" }},
		{"missing key", "UPDATE_UNAVAILABLE_BUILD", func(f *fixture) { f.provider.PublicKey = nil }},
		{"wrong platform", "UPDATE_ASSET_MISSING", func(f *fixture) { f.filename = ArtifactName("windows", "amd64") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			tc.mutate(f)
			_, e := f.provider.Check(context.Background(), request())
			if e == nil || !strings.Contains(e.Error(), tc.code) {
				t.Fatalf("want %s, got %v", tc.code, e)
			}
		})
	}
}
func TestOldReleaseAndInvalidVersion(t *testing.T) {
	f := newFixture(t)
	f.tag = "v0.9.0"
	r, e := f.provider.Check(context.Background(), request())
	if r != nil || e != nil {
		t.Fatalf("old release %v %v", r, e)
	}
	for _, v := range []string{"1.0.0-beta", "1.02.0", "999999999999999999999999999.0.0", "1.0", "1.0.0;bad"} {
		if ValidVersion(v) {
			t.Fatal(v)
		}
	}
	if !newer("1.10.0", "1.9.9") || newer("1.0.0", "1.0.0") {
		t.Fatal("version order")
	}
}
func TestDownloadSizeMismatch(t *testing.T) {
	for _, extra := range []bool{true, false} {
		f := newFixture(t)
		r, e := f.provider.Check(context.Background(), request())
		if e != nil {
			t.Fatal(e)
		}
		if extra {
			f.artifact = append(f.artifact, 0)
		} else {
			f.artifact = f.artifact[:len(f.artifact)-1]
		}
		if e = f.provider.Download(context.Background(), r, &bytes.Buffer{}, nil); e == nil {
			t.Fatal("accepted wrong size")
		}
	}
}

type testHost struct{}

func (testHost) Emit(string, ...any) bool         { return false }
func (testHost) OnEvent(string, func(any)) func() { return func() {} }
func (testHost) OpenWindow(updater.WindowOptions) updater.WindowHandle {
	panic("unexpected updater UI")
}
func (testHost) Quit() { panic("test must not replace running executable") }
func TestWailsVerifiesAndStagesSignedArchive(t *testing.T) {
	// Wails stages in the system temp directory. Isolate all files from the application.
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("TMP", t.TempDir())
	t.Setenv("TEMP", t.TempDir())
	for _, tamper := range []bool{false, true} {
		f := newFixture(t)
		if tamper {
			f.artifact = bytes.Clone(f.artifact)
			f.artifact[len(f.artifact)/2] ^= 1
		}
		u := updater.New(testHost{})
		e := u.Init(updater.Config{CurrentVersion: "1.0.0", Providers: []updater.Provider{f.provider}, PublicKey: f.provider.PublicKey, Platform: "linux", Arch: "amd64", Window: updater.WindowNone})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = u.Check(context.Background()); e != nil {
			t.Fatal(e)
		}
		e = u.DownloadAndInstall(context.Background())
		if tamper {
			if e == nil {
				t.Fatal("Wails accepted modified artifact")
			}
		} else if e != nil || u.State() != updater.StateReady {
			t.Fatalf("staging failed: %v (%s)", e, u.State())
		}
	}
}
