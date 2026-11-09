//go:build ignore

// Build a portable package for the current operating system from the repository root.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"tunnelix/internal/buildinfo"
	"tunnelix/internal/release"
	"tunnelix/internal/updates"
)

func main() {
	production := flag.Bool("release", false, "require signed production release inputs")
	allowUnsigned := flag.Bool("allow-unsigned", false, "allow preview packages without OS signing; requires -release and update signing keys")
	flag.Parse()
	if *allowUnsigned && !*production {
		panic("allow-unsigned requires -release")
	}
	if !release.ValidTarget(runtime.GOOS, runtime.GOARCH) {
		panic("unsupported release target")
	}
	version := os.Getenv("TUNNELIX_VERSION")
	if version == "" {
		version = buildinfo.Version
	}
	if !updates.ValidVersion(version) {
		panic("TUNNELIX_VERSION must be x.y.z")
	}
	public := os.Getenv("TUNNELIX_UPDATE_PUBLIC_KEY")
	private := os.Getenv("TUNNELIX_UPDATE_PRIVATE_KEY")
	must(os.Unsetenv("TUNNELIX_UPDATE_PRIVATE_KEY")) // Never inherit the signing key in build subprocesses.
	var signingKey ed25519.PrivateKey
	if public != "" {
		key, err := base64.StdEncoding.DecodeString(public)
		must(err)
		if len(key) != ed25519.PublicKeySize {
			panic("invalid update public key")
		}
	}
	if private != "" {
		key, err := base64.StdEncoding.DecodeString(private)
		must(err)
		if len(key) != ed25519.PrivateKeySize {
			panic("invalid update private key")
		}
		signingKey = ed25519.PrivateKey(key)
		if public == "" || base64.StdEncoding.EncodeToString(signingKey.Public().(ed25519.PublicKey)) != public {
			panic("update signing keys do not match")
		}
	}
	commit := strings.TrimSpace(output("git", "rev-parse", "HEAD"))
	dirty := strings.TrimSpace(output("git", "status", "--porcelain")) != ""
	identity := os.Getenv("TUNNELIX_MAC_SIGN_IDENTITY")
	profile := os.Getenv("TUNNELIX_MAC_NOTARY_PROFILE")
	certificate := os.Getenv("TUNNELIX_WINDOWS_CERTIFICATE")
	if *production {
		if dirty {
			panic("release requires a clean Git checkout")
		}
		if signingKey == nil {
			panic("release requires update signing keys")
		}
		if !*allowUnsigned && runtime.GOOS == "darwin" && (identity == "" || identity == "-" || profile == "") {
			panic("release requires Developer ID signing and notarization")
		}
		if !*allowUnsigned && runtime.GOOS == "windows" && certificate == "" {
			panic("release requires a Windows signing certificate")
		}
	}
	command("go", "run", "scripts/licenses.go")
	outDir := filepath.Join("dist", version, runtime.GOOS+"-"+runtime.GOARCH)
	must(os.RemoveAll(outDir)) // Only the validated version/platform generated output is replaced.
	must(os.MkdirAll(outDir, 0755))
	base := release.BaseName(version, runtime.GOOS, runtime.GOARCH)
	var files []string
	add := func(name string) string { files = append(files, name); return filepath.Join(outDir, name) }
	osSigned, notarized := false, false
	must(os.MkdirAll("bin", 0755))
	name := "tunnelix"
	flags := "-s -w -X tunnelix/internal/buildinfo.Version=" + version + " -X tunnelix/internal/buildinfo.UpdatePublicKey=" + public + " -X tunnelix/internal/buildinfo.Commit=" + commit
	if runtime.GOOS == "windows" {
		name += ".exe"
		flags += " -H windowsgui"
	}
	command("go", "build", "-trimpath", "-ldflags", flags, "-o", filepath.Join("bin", name), ".")
	stage := filepath.Join("build", runtime.GOOS+"-"+runtime.GOARCH)
	must(os.RemoveAll(stage))
	must(os.MkdirAll(stage, 0755))
	switch runtime.GOOS {
	case "darwin":
		root := filepath.Join(stage, "Tunnelix.app", "Contents")
		must(os.MkdirAll(filepath.Join(root, "MacOS"), 0755))
		must(os.MkdirAll(filepath.Join(root, "Resources"), 0755))
		copyFile(filepath.Join("bin", name), filepath.Join(root, "MacOS", "Tunnelix"), 0755)
		plist := `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>CFBundleName</key><string>Tunnelix</string><key>CFBundleDisplayName</key><string>Tunnelix</string><key>CFBundleIdentifier</key><string>app.tunnelix.desktop</string><key>CFBundleExecutable</key><string>Tunnelix</string><key>CFBundleIconFile</key><string>appicon</string><key>CFBundlePackageType</key><string>APPL</string><key>CFBundleShortVersionString</key><string>` + version + `</string><key>CFBundleVersion</key><string>` + version + `</string><key>LSMinimumSystemVersion</key><string>14.0</string><key>NSHighResolutionCapable</key><true/></dict></plist>`
		must(os.WriteFile(filepath.Join(root, "Info.plist"), []byte(plist), 0644))
		iconset := filepath.Join(stage, "appicon.iconset")
		must(os.MkdirAll(iconset, 0755))
		for _, size := range []int{16, 32, 128, 256, 512} {
			for _, scale := range []int{1, 2} {
				suffix := ""
				if scale == 2 {
					suffix = "@2x"
				}
				out := filepath.Join(iconset, fmt.Sprintf("icon_%dx%d%s.png", size, size, suffix))
				command("sips", "-z", fmt.Sprint(size*scale), fmt.Sprint(size*scale), "assets/brand/icon.png", "--out", out)
			}
		}
		command("iconutil", "-c", "icns", iconset, "-o", filepath.Join(root, "Resources", "appicon.icns"))
		copyFile("THIRD_PARTY_LICENSES.txt", filepath.Join(root, "Resources", "THIRD_PARTY_LICENSES.txt"), 0644)
		copyFile("LICENSE", filepath.Join(root, "Resources", "LICENSE"), 0644)
		copyFile("THIRD_PARTY_NOTICES.md", filepath.Join(root, "Resources", "THIRD_PARTY_NOTICES.md"), 0644)
		packageDocs(filepath.Join(root, "Resources"))
		appPath := filepath.Join(stage, "Tunnelix.app")
		if identity != "" && identity != "-" {
			command("codesign", "--force", "--options", "runtime", "--timestamp", "--sign", identity, appPath)
			command("codesign", "--verify", "--deep", "--strict", appPath)
			osSigned = true
		} else {
			command("codesign", "--force", "--sign", "-", appPath)
		}
		if profile != "" {
			if !osSigned {
				panic("notarization requires Developer ID signing")
			}
			submission := filepath.Join(stage, "notarize.zip")
			zipDir(appPath, submission)
			notarize(submission, profile)
			command("xcrun", "stapler", "staple", appPath)
			command("xcrun", "stapler", "validate", appPath)
			notarized = true
		}
		portable := add(base + ".zip")
		zipDir(appPath, portable)
		extracted := filepath.Join(stage, "zip-verification")
		command("ditto", "-x", "-k", portable, extracted)
		command("codesign", "--verify", "--deep", "--strict", filepath.Join(extracted, "Tunnelix.app"))
		if notarized {
			command("xcrun", "stapler", "validate", filepath.Join(extracted, "Tunnelix.app"))
		}
		must(os.RemoveAll(extracted))
		dmgRoot := filepath.Join(stage, "dmg")
		must(os.MkdirAll(dmgRoot, 0755))
		command("ditto", appPath, filepath.Join(dmgRoot, "Tunnelix.app"))
		must(os.Symlink("/Applications", filepath.Join(dmgRoot, "Applications")))
		copyFile("docs/install.md", filepath.Join(dmgRoot, "Installation.md"), 0644)
		dmg := add(base + ".dmg")
		command("hdiutil", "create", "-volname", "Tunnelix", "-srcfolder", dmgRoot, "-ov", "-format", "UDZO", dmg)
		if osSigned {
			command("codesign", "--force", "--timestamp", "--sign", identity, dmg)
		}
		if notarized {
			notarize(dmg, profile)
			command("xcrun", "stapler", "staple", dmg)
			command("xcrun", "stapler", "validate", dmg)
		}
	case "windows", "linux":
		root := filepath.Join(stage, "Tunnelix")
		must(os.MkdirAll(root, 0755))
		if runtime.GOOS == "windows" {
			writeICO(filepath.Join(root, "tunnelix.ico"))
		}
		if runtime.GOOS == "windows" && certificate != "" {
			signWindows(filepath.Join("bin", name))
			osSigned = true
		}
		copyFile(filepath.Join("bin", name), filepath.Join(root, name), 0755)
		for _, f := range []string{"LICENSE", "THIRD_PARTY_NOTICES.md", "README.md", "README.zh-CN.md"} {
			copyFile(f, filepath.Join(root, f), 0644)
		}
		copyFile("THIRD_PARTY_LICENSES.txt", filepath.Join(root, "THIRD_PARTY_LICENSES.txt"), 0644)
		copyFile("assets/brand/icon.png", filepath.Join(root, "tunnelix.png"), 0644)
		packageDocs(root)
		if runtime.GOOS == "linux" {
			must(os.WriteFile(filepath.Join(root, "tunnelix.desktop"), []byte("[Desktop Entry]\nType=Application\nName=Tunnelix\nComment=SSH tunnel manager\nExec=tunnelix\nIcon=tunnelix\nTerminal=false\nCategories=Network;Utility;\n"), 0644))
			tarGz(root, add(base+".tar.gz"))
		} else {
			zipDir(root, add(base+".zip"))
			compiler, err := exec.LookPath("makensis")
			if err != nil && *production {
				panic("release requires NSIS (makensis)")
			}
			if err == nil {
				icon := filepath.Join(root, "tunnelix.ico")
				installer := add(base + "-setup.exe")
				rootAbs, err := filepath.Abs(root)
				must(err)
				outputAbs, err := filepath.Abs(installer)
				must(err)
				iconAbs, err := filepath.Abs(icon)
				must(err)
				args := []string{"/DPRODUCT_VERSION=" + version, "/DPACKAGE_DIR=" + rootAbs, "/DOUTPUT_FILE=" + outputAbs, "/DICON_FILE=" + iconAbs}
				if osSigned {
					script, err := filepath.Abs("scripts/sign-windows.ps1")
					must(err)
					args = append(args, "/DSIGN_SCRIPT="+script)
				}
				command(compiler, append(args, "scripts/installer.nsi")...)
				if osSigned {
					signWindows(installer)
				}
			} else {
				fmt.Println("NSIS unavailable: portable ZIP generated; installer skipped.")
			}
		}
	default:
		panic("unsupported platform")
	}
	updateName := updates.ArtifactName(runtime.GOOS, runtime.GOARCH)
	updateFile := add(updateName)
	updateRoot := filepath.Join("bin", name)
	if runtime.GOOS == "darwin" {
		updateRoot = filepath.Join(stage, "Tunnelix.app")
	}
	zipDir(updateRoot, updateFile)
	// A stale signature must never accompany a freshly rebuilt artifact.
	if err := os.Remove(updateFile + ".update.json"); err != nil && !os.IsNotExist(err) {
		must(err)
	}
	if signingKey != nil {
		data, err := os.ReadFile(updateFile)
		must(err)
		manifest, err := updates.SignedManifest(version, filepath.Base(updateFile), data, signingKey)
		must(err)
		must(os.WriteFile(add(updateName+".update.json"), manifest, 0644))
	}
	must(release.Write(outDir, release.Manifest{Version: version, Platform: runtime.GOOS, Arch: runtime.GOARCH, Commit: commit, Dirty: dirty, OSSigned: osSigned, Notarized: notarized, UpdatePublicKey: public}, files))
	if signingKey != nil || public == "" {
		_, err := release.Verify(outDir, version, public, release.VerifyOptions{Production: *production, AllowUnsigned: *allowUnsigned})
		must(err)
	}
	fmt.Println("Packages ready in " + outDir)
}
func must(e error) {
	if e != nil {
		panic(e)
	}
}
func command(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	must(cmd.Run())
}
func copyFile(src, dst string, mode fs.FileMode) {
	b, e := os.ReadFile(src)
	must(e)
	must(os.WriteFile(dst, b, mode))
}
func zipDir(root, out string) {
	f, e := os.Create(out)
	must(e)
	z := zip.NewWriter(f)
	must(filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		h, e := zip.FileInfoHeader(info)
		if e != nil {
			return e
		}
		h.Name = filepath.ToSlash(strings.TrimPrefix(path, filepath.Dir(root)+string(os.PathSeparator)))
		h.Method = zip.Deflate
		w, e := z.CreateHeader(h)
		if e != nil {
			return e
		}
		r, e := os.Open(path)
		if e != nil {
			return e
		}
		defer r.Close()
		_, e = io.Copy(w, r)
		return e
	}))
	must(z.Close())
	must(f.Close())
}
func tarGz(root, out string) {
	f, e := os.Create(out)
	must(e)
	g := gzip.NewWriter(f)
	w := tar.NewWriter(g)
	must(filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		h, e := tar.FileInfoHeader(info, "")
		if e != nil {
			return e
		}
		h.Name = filepath.ToSlash(strings.TrimPrefix(path, filepath.Dir(root)+string(os.PathSeparator)))
		if e = w.WriteHeader(h); e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		r, e := os.Open(path)
		if e != nil {
			return e
		}
		defer r.Close()
		_, e = io.Copy(w, r)
		return e
	}))
	must(w.Close())
	must(g.Close())
	must(f.Close())
}

func output(name string, args ...string) string {
	b, err := exec.Command(name, args...).Output()
	must(err)
	return string(b)
}
func notarize(path, profile string) {
	args := []string{"notarytool", "submit", path, "--keychain-profile", profile, "--wait", "--timeout", "20m"}
	if keychain := os.Getenv("TUNNELIX_MAC_KEYCHAIN"); keychain != "" {
		args = append(args, "--keychain", keychain)
	}
	command("xcrun", args...)
}
func signWindows(path string) {
	command("powershell.exe", "-NoProfile", "-NonInteractive", "-File", "scripts/sign-windows.ps1", path)
}
func writeICO(path string) {
	f, err := os.Open("assets/brand/icon.png")
	must(err)
	src, err := png.Decode(f)
	must(err)
	must(f.Close())
	img := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			img.Set(x, y, src.At(src.Bounds().Min.X+x*src.Bounds().Dx()/256, src.Bounds().Min.Y+y*src.Bounds().Dy()/256))
		}
	}
	var pixels bytes.Buffer
	must(png.Encode(&pixels, img))
	// PNG-backed ICO entries are supported by all supported Windows versions.
	var ico bytes.Buffer
	for _, v := range []uint16{0, 1, 1} {
		must(binary.Write(&ico, binary.LittleEndian, v))
	}
	ico.Write([]byte{0, 0, 0, 0})
	for _, v := range []uint16{1, 32} {
		must(binary.Write(&ico, binary.LittleEndian, v))
	}
	must(binary.Write(&ico, binary.LittleEndian, uint32(pixels.Len())))
	must(binary.Write(&ico, binary.LittleEndian, uint32(22)))
	ico.Write(pixels.Bytes())
	must(os.WriteFile(path, ico.Bytes(), 0644))
}

func packageDocs(root string) {
	must(os.MkdirAll(filepath.Join(root, "docs"), 0755))
	for _, name := range []string{"install.md", "development.md", "releasing.md"} {
		copyFile(filepath.Join("docs", name), filepath.Join(root, "docs", name), 0644)
	}
	for _, name := range []string{"README.md", "README.zh-CN.md"} {
		copyFile(name, filepath.Join(root, name), 0644)
	}
}
