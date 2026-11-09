package updates

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRestartReplacesIsolatedExecutable(t *testing.T) {
	dir := t.TempDir()
	name := "tunnelix-update-test"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	old := filepath.Join(dir, name)
	next := filepath.Join(dir, "next", name)
	marker := filepath.Join(dir, "restarted")
	for _, b := range []struct{ path, version string }{{old, "1.0.0"}, {next, "2.0.0"}} {
		cmd := exec.Command("go", "build", "-ldflags", "-X main.version="+b.version, "-o", b.path, "./testdata/restart")
		if out, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("fixture build: %v %s", e, out)
		}
	}
	payload, e := os.ReadFile(next)
	if e != nil {
		t.Fatal(e)
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: name, Method: zip.Deflate}
	h.SetMode(0755)
	w, e := z.CreateHeader(h)
	if e != nil {
		t.Fatal(e)
	}
	w.Write(payload)
	z.Close()
	pub, priv, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	filename := ArtifactName(runtime.GOOS, runtime.GOARCH)
	manifest, e := SignedManifest("2.0.0", filename, buf.Bytes(), priv)
	if e != nil {
		t.Fatal(e)
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/al6nlee/tunnelix/releases/latest":
			json.NewEncoder(w).Encode(map[string]any{"tag_name": "v2.0.0", "assets": []map[string]string{{"name": filename, "browser_download_url": "http://" + r.Host + "/artifact"}, {"name": filename + ".update.json", "browser_download_url": "http://" + r.Host + "/manifest"}}})
		case "/manifest":
			w.Write(manifest)
		case "/artifact":
			w.Write(buf.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, old)
	cmd.Env = append(os.Environ(), "TUNNELIX_TEST_RESTART_MARKER="+marker, "TUNNELIX_TEST_UPDATE_KEY="+base64.StdEncoding.EncodeToString(pub), "TUNNELIX_TEST_UPDATE_URL="+server.URL)
	log, e := os.Create(filepath.Join(dir, "run.log"))
	if e != nil {
		t.Fatal(e)
	}
	defer log.Close()
	cmd.Stdout = log
	cmd.Stderr = log
	if e = cmd.Run(); e != nil {
		out, _ := os.ReadFile(log.Name())
		t.Fatalf("restart launch: %v %s", e, out)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(marker); err == nil && string(b) == "2.0.0" {
			actual, err := os.ReadFile(old)
			if err != nil || !bytes.Equal(actual, payload) {
				t.Fatal("target was not replaced")
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	out, _ := os.ReadFile(log.Name())
	t.Fatalf("new version did not relaunch: %s", out)
}
