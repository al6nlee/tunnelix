# Third-party dependencies

Tunnelix is Copyright (c) 2026 Alan and is distributed under the MIT License.
The supplied Tunnelix brand artwork is maintained in `assets/brand/`.

Tunnelix uses open-source dependencies, including Wails, React, Vite, TypeScript,
Tailwind CSS, Radix UI, i18next, react-i18next, Lucide, go-winio, and Go libraries.
Their respective licenses apply. Dependency versions are pinned in `go.mod`,
`go.sum`, `frontend/package.json`, and `frontend/package-lock.json`.

[THIRD_PARTY_LICENSES.txt](THIRD_PARTY_LICENSES.txt) in the repository root contains
the license and notice files for the Go toolchain, the Go modules declared in
go.mod across supported targets, and the installed frontend packages used by
the build. Keep this file in version control and regenerate it with
`go run scripts/licenses.go` after dependency updates. Packaging regenerates the
file and includes it in the portable package. OpenSSH is an external runtime
dependency and is not bundled.
