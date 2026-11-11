package updates

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"tunnelix/internal/buildinfo"
)

const MaxArtifactSize int64 = 256 << 20

const githubAPIBase = "https://api.github.com"

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func ValidVersion(v string) bool {
	if !versionPattern.MatchString(v) {
		return false
	}
	for _, component := range strings.Split(v, ".") {
		if _, err := strconv.ParseUint(component, 10, 64); err != nil {
			return false
		}
	}
	return true
}
func newer(a, b string) bool {
	if !ValidVersion(a) || !ValidVersion(b) {
		return false
	}
	aa, bb := strings.Split(a, "."), strings.Split(b, ".")
	for i := range aa {
		x, _ := strconv.ParseUint(aa[i], 10, 64)
		y, _ := strconv.ParseUint(bb[i], 10, 64)
		if x != y {
			return x > y
		}
	}
	return false
}
func ArtifactName(platform, arch string) string {
	return fmt.Sprintf("Tunnelix-update-%s-%s.zip", platform, arch)
}

type Manifest struct {
	Version           string `json:"version"`
	Filename          string `json:"filename"`
	Size              int64  `json:"size"`
	SHA256            string `json:"sha256"`
	ArtifactSignature []byte `json:"artifactSignature"`
}
type Envelope struct {
	Payload   []byte `json:"payload"`
	Signature []byte `json:"signature"`
}

func SignedManifest(version, filename string, data []byte, key ed25519.PrivateKey) ([]byte, error) {
	if !ValidVersion(version) || len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid signing inputs")
	}
	sum := sha256.Sum256(data)
	m := Manifest{version, filename, int64(len(data)), hex.EncodeToString(sum[:]), ed25519.Sign(key, sum[:])}
	payload, e := json.Marshal(m)
	if e != nil {
		return nil, e
	}
	return json.MarshalIndent(Envelope{payload, ed25519.Sign(key, payload)}, "", "  ")
}

type Provider struct {
	OnProgress func(int64, int64)
	Client     *http.Client
	BaseURL    string
	PublicKey  ed25519.PublicKey
}

func NewProvider() (*Provider, error) {
	var key []byte
	var e error
	if buildinfo.UpdatePublicKey != "" {
		key, e = base64.StdEncoding.DecodeString(buildinfo.UpdatePublicKey)
		if e != nil || len(key) != ed25519.PublicKeySize {
			return nil, errors.New("UPDATE_UNAVAILABLE_BUILD")
		}
	}
	return &Provider{Client: &http.Client{Timeout: 20 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || req.URL.Scheme != "https" {
			return errors.New("UPDATE_DOWNLOAD_FAILED")
		}
		return nil
	}}, BaseURL: "https://api.github.com", PublicKey: key}, nil
}
func (p *Provider) Name() string { return "tunnelix-releases" }
func (p *Provider) get(ctx context.Context, address string, max int64) ([]byte, int, error) {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if e != nil {
		return nil, 0, e
	}
	req.Header.Set("User-Agent", "Tunnelix/"+buildinfo.Version)
	response, e := p.Client.Do(req)
	if e != nil {
		return nil, 0, e
	}
	defer response.Body.Close()
	data, e := io.ReadAll(io.LimitReader(response.Body, max+1))
	if len(data) > int(max) {
		return nil, response.StatusCode, errors.New("response too large")
	}
	return data, response.StatusCode, e
}
func (p *Provider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	data, status, e := p.get(ctx, p.BaseURL+"/repos/"+buildinfo.Repository+"/releases/latest", 2<<20)
	fallback := false
	if p.BaseURL == githubAPIBase && (e != nil || status != http.StatusOK) {
		// GitHub's unauthenticated REST API is limited per source IP. The
		// public /releases/latest redirect is not subject to that API quota.
		version, fallbackErr := p.latestVersionFromWeb(ctx)
		if fallbackErr != nil {
			return nil, fallbackErr
		}
		data, e = json.Marshal(map[string]string{"tag_name": "v" + version})
		status = http.StatusOK
		fallback = true
	}
	if e != nil {
		return nil, errors.New("UPDATE_CHECK_FAILED")
	}
	if status == http.StatusNotFound {
		return nil, errors.New("UPDATE_NO_RELEASE")
	}
	if status != http.StatusOK {
		return nil, errors.New("UPDATE_CHECK_FAILED")
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Notes      string `json:"body"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if json.Unmarshal(data, &release) != nil {
		return nil, errors.New("UPDATE_CHECK_FAILED")
	}
	version := strings.TrimPrefix(release.Tag, "v")
	if !ValidVersion(req.CurrentVersion) || !ValidVersion(version) || release.Draft || release.Prerelease {
		return nil, errors.New("UPDATE_CHECK_FAILED")
	}
	if !newer(version, req.CurrentVersion) {
		return nil, nil
	}
	if len(p.PublicKey) != ed25519.PublicKeySize {
		return nil, errors.New("UPDATE_UNAVAILABLE_BUILD")
	}
	filename := ArtifactName(req.Platform, req.Arch)
	urls := map[string]string{}
	for _, asset := range release.Assets {
		urls[asset.Name] = asset.URL
	}
	artifactURL, manifestURL := urls[filename], urls[filename+".update.json"]
	if fallback {
		artifactURL = fmt.Sprintf("https://github.com/%s/releases/download/v%s/%s", buildinfo.Repository, version, filename)
		manifestURL = artifactURL + ".update.json"
	}
	if !p.allowed(artifactURL) || !p.allowed(manifestURL) {
		return nil, errors.New("UPDATE_ASSET_MISSING")
	}
	body, status, e := p.get(ctx, manifestURL, 64<<10)
	if e != nil || status != 200 {
		return nil, errors.New("UPDATE_CHECK_FAILED")
	}
	var envelope Envelope
	if json.Unmarshal(body, &envelope) != nil || !ed25519.Verify(p.PublicKey, envelope.Payload, envelope.Signature) {
		return nil, errors.New("UPDATE_VERIFY_FAILED")
	}
	var manifest Manifest
	if json.Unmarshal(envelope.Payload, &manifest) != nil || manifest.Version != version || manifest.Filename != filename || manifest.Size <= 0 || manifest.Size > MaxArtifactSize {
		return nil, errors.New("UPDATE_VERIFY_FAILED")
	}
	digest, e := hex.DecodeString(manifest.SHA256)
	if e != nil || len(digest) != 32 || !ed25519.Verify(p.PublicKey, digest, manifest.ArtifactSignature) {
		return nil, errors.New("UPDATE_VERIFY_FAILED")
	}
	return &updater.Release{Version: version, Notes: release.Notes, Artifact: updater.Artifact{Filename: filename, Filetype: "zip", Size: manifest.Size, Platform: req.Platform, Arch: req.Arch}, Verification: &updater.Verification{DigestAlgo: "sha256", Digest: digest, SignatureAlgo: "ed25519", Signature: manifest.ArtifactSignature}, Metadata: map[string]any{"url": artifactURL}}, nil
}

func (p *Provider) latestVersionFromWeb(ctx context.Context) (string, error) {
	request, e := http.NewRequestWithContext(ctx, http.MethodGet, "https://github.com/"+buildinfo.Repository+"/releases/latest", nil)
	if e != nil {
		return "", errors.New("UPDATE_CHECK_FAILED")
	}
	request.Header.Set("User-Agent", "Tunnelix/"+buildinfo.Version)
	client := &http.Client{Timeout: p.Client.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	response, e := client.Do(request)
	if e != nil {
		return "", errors.New("UPDATE_CHECK_FAILED")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return "", errors.New("UPDATE_NO_RELEASE")
	}
	if response.StatusCode < http.StatusMultipleChoices || response.StatusCode >= http.StatusBadRequest {
		return "", errors.New("UPDATE_CHECK_FAILED")
	}
	version, ok := versionFromReleaseLocation(response.Header.Get("Location"))
	if !ok {
		return "", errors.New("UPDATE_CHECK_FAILED")
	}
	return version, nil
}

func versionFromReleaseLocation(location string) (string, bool) {
	u, e := url.Parse(location)
	if e != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil {
		return "", false
	}
	prefix := "/" + buildinfo.Repository + "/releases/tag/"
	if !strings.HasPrefix(u.Path, prefix) {
		return "", false
	}
	version := strings.TrimPrefix(strings.TrimSuffix(strings.TrimPrefix(u.Path, prefix), "/"), "v")
	if !ValidVersion(version) {
		return "", false
	}
	return version, true
}

func (p *Provider) allowed(address string) bool {
	u, e := url.Parse(address)
	if e != nil {
		return false
	}
	// Test servers are explicit injected instances, never configurable by the UI.
	if strings.HasPrefix(p.BaseURL, "http://127.0.0.1:") {
		return strings.HasPrefix(address, p.BaseURL+"/")
	}
	return u.Scheme == "https" && u.Host == "github.com" && u.User == nil && strings.HasPrefix(u.Path, "/"+buildinfo.Repository+"/releases/download/")
}
func (p *Provider) Download(ctx context.Context, r *updater.Release, dst io.Writer, progress func(int64, int64)) error {
	address, _ := r.Metadata["url"].(string)
	if !p.allowed(address) {
		return errors.New("UPDATE_DOWNLOAD_FAILED")
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if e != nil {
		return e
	}
	response, e := p.Client.Do(req)
	if e != nil {
		return errors.New("UPDATE_DOWNLOAD_FAILED")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return errors.New("UPDATE_DOWNLOAD_FAILED")
	}
	reader := io.LimitReader(response.Body, r.Artifact.Size+1)
	buffer := make([]byte, 64<<10)
	var written int64
	digest := sha256.New()
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			written += int64(n)
			if written > r.Artifact.Size {
				return errors.New("UPDATE_VERIFY_FAILED")
			}
			digest.Write(buffer[:n])
			if _, e = dst.Write(buffer[:n]); e != nil {
				return e
			}
			if progress != nil {
				progress(written, r.Artifact.Size)
			}
			if p.OnProgress != nil {
				p.OnProgress(written, r.Artifact.Size)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.New("UPDATE_DOWNLOAD_FAILED")
		}
	}
	if written != r.Artifact.Size || r.Verification == nil || !bytes.Equal(digest.Sum(nil), r.Verification.Digest) {
		return errors.New("UPDATE_VERIFY_FAILED")
	}
	return nil
}
