# Releasing

Publishing is the owner's decision. Agents never push a release tag, never publish a draft and
run `scripts/release.ps1` only with `-DryRun`.

A release is built by GitHub Actions for both platforms (`.github/workflows/release.yml`) and
lands as a **draft** in the releases repo `econ11/marstek-monitor`. The owner publishes the draft.
`scripts/release.ps1` is an emergency, Windows-only fallback.

## Assets

The in-app updater depends on these names; they must not change:

- `marstek-monitor-<version>-setup.exe` and `marstek-monitor-<version>-setup.exe.sha256`
- `marstek-monitor-<version>-macos.dmg` and `marstek-monitor-<version>-macos.dmg.sha256`

Each `.sha256` is one line `<hex>  <filename>` (two spaces, LF), as `sha256sum` prints it.

## Secrets (Actions secrets of `econ11/marstek-monitor-code`)

Names only; the values live only in the repo's Actions secrets, set by the owner from files
(`gh secret set NAME < file`), never typed into a chat or a log. Setup: `docs/macos-signing.md`.

| Secret | What |
| --- | --- |
| `MACOS_CERT_P12` | the Developer ID Application certificate with its private key, `.p12`, base64-encoded |
| `MACOS_CERT_PASSWORD` | the `.p12` export password |
| `NOTARY_KEY_ID` | App Store Connect API key ID |
| `NOTARY_ISSUER` | App Store Connect issuer ID |
| `NOTARY_KEY_P8` | the API key file `AuthKey_<id>.p8`, its text as is |
| `RELEASES_TOKEN` | fine-grained token, `contents: write` on `econ11/marstek-monitor` only |

The macOS job fails at its first step if any of the five signing secrets is missing: an unsigned
or un-notarized dmg is never built for a release. The publish job fails without `RELEASES_TOKEN`.

## CI

`.github/workflows/ci.yml` runs on every push and pull request on `windows-latest` and
`macos-latest`: `npm --prefix frontend ci`, `wails3 build` (generates the gitignored
`frontend/bindings` and `frontend/dist`), `go vet ./...`, `go test ./...`,
`npm --prefix frontend test`, `npm --prefix frontend run typecheck`, and on macOS also
`go test -tags release ./core/update/`.

## The release: tag → draft → publish

1. `CHANGELOG.md` on `main` has a non-empty `## <version>` section (it becomes the notes;
   `scripts/release-notes.ps1 -Version <version>` prints it). The version is `x.y.z`: a version
   with `-pre`, `-rc` or `+build` is rejected.
2. Optional rehearsal: Actions → **release** → *Run workflow* with `version=<version>` and
   `dry_run` checked (the default). Everything is built, signed and checked; nothing is published.
3. Tag and push (owner):

   ```powershell
   git tag -a v<version> -m "Marstek Monitor <version>"
   git push origin v<version>
   ```

4. The workflow runs:
   - **prepare**: checks the version (`x.y.z`) and the CHANGELOG section;
   - **windows**: the checks, plus `go test -tags release ./core/update/`;
     `wails3 task package VERSION=<version>` with NSIS 3.13 (the Taskfile adds the `release`
     build tag); `go version -m` must show the `release` tag on `bin/marstek-monitor.exe`;
     writes the `.sha256`;
   - **macos**: the signing-secrets check first; the checks; a temporary keychain with the
     certificate; the API key in `$RUNNER_TEMP`;
     `scripts/macos-package.sh <version> "Developer ID Application"` (universal app, hardened
     runtime, notarized and stapled app and dmg); `spctl --assess --type execute` on the app
     inside the dmg; the keychain and the key are deleted in an `always()` step;
   - **intel-check** (Intel runner `macos-15-intel`): the app in the dmg has both `arm64` and
     `x86_64`, and `codesign --verify --deep --strict` and `spctl --assess` pass;
   - **publish** (needs all of the above; skipped on a dry run): fails if `v<version>` is already
     **published** in `econ11/marstek-monitor`; deletes an existing **draft** of it (a re-run
     replaces the draft); then `gh release create v<version> --repo econ11/marstek-monitor
     --draft --target main --title "Marstek Monitor <version>" --notes-file …` with the four
     assets.
5. The owner reviews the draft in `econ11/marstek-monitor` (notes, four assets) and publishes it.
   Publishing creates the tag `v<version>` in the releases repo; until then the apps see nothing.
6. After publishing: install the setup on a Windows machine and the dmg on a Mac, and check that
   "Check for updates" in the previous version offers it.

A failed run publishes nothing. Fix the cause on `main`, then either re-run the workflow, or delete
the code-repo tag, tag the fixed commit and push the tag again.

## Emergency fallback: `scripts/release.ps1` (Windows only)

Only when the workflow cannot run. It builds and publishes the Windows installer from this PC,
directly as a published release (no draft). **Such a release has no macOS dmg, so Mac users get
no update offer for that version**; they stay on their version until the next full release.

Prerequisites: clean working tree on `main` (override with `-Branch`); the CHANGELOG section; Go,
Node, `wails3` and NSIS 3 (`makensis` on PATH or in `C:\Program Files (x86)\NSIS`); `gh`
authenticated with write access to `econ11/marstek-monitor`.

```powershell
scripts/release.ps1 -Version <version> -DryRun
```

The dry run does everything except create the tag and the GitHub release, which are only printed:

1. clean tree on the release branch;
2. `v<version>` is not tagged yet;
3. CHANGELOG has `## <version>` (`scripts/release-notes.ps1`);
4. `wails3 build`: installs the frontend dependencies and generates `frontend/bindings` and
   `frontend/dist` (all gitignored, so a fresh clone needs this before `go vet` and the typecheck);
5. `go vet ./...`, `go test ./...`, `go test -tags release ./core/update/`,
   `npm --prefix frontend test`, `npm --prefix frontend run typecheck`;
6. `wails3 task package VERSION=<version>` produces `bin\marstek-monitor-<version>-setup.exe`, and
   `go version -m bin\marstek-monitor.exe` must show the `release` build tag;
7. SHA-256 goes to `bin\marstek-monitor-<version>-setup.exe.sha256`; the notes go to
   `bin\release-notes-<version>.md`;
8. re-check that the build left no tracked file changed (`git diff --quiet HEAD`, content-based:
   with `core.autocrlf=true` a same-content rewrite such as `go mod tidy` in a fresh clone is not a
   change);
9. `gh release create v<version> --repo econ11/marstek-monitor --title "Marstek Monitor <version>"
   --notes-file ... <installer> <installer>.sha256` (a `-rc`/`-pre` version adds `--prerelease`);
   only after it succeeds the local annotated tag `v<version>` is created (dry run: printed only).

Without `-DryRun` the script runs steps 1-8, then asks you to type `publish v<version>`. Anything
else aborts and no tag is created; a failed `gh` leaves no tag either. The script never pushes the
local tag. Pushing it to the code repo would start the release workflow, whose publish job then
fails because the release is already published.

## First Windows release only: the owner's PC

A hand-copied dev build never checks for updates, so move to the installed app by hand. Quit the
running hand-copied app (tray → «Вийти»), run the published `marstek-monitor-<version>-setup.exe`
interactively with the default folder (`%LOCALAPPDATA%\Programs\Marstek Monitor`, not the
hand-copy folder), start it, and check that «Про програму» shows `<version>` and that HKCU Run
`MarstekMonitor` points to the installed exe. Then the hand copy can be deleted. Firewall: the
installed exe has a new path, so Windows may ask for network access again; allow it on
**private** networks, or the app won't find the station (the app's own diagnosis offers the rule
otherwise).

## Tests

`Invoke-Pester scripts` runs `release-notes.Tests.ps1` (the CHANGELOG section parser and the
script's output file) and `release.Tests.ps1` (the fallback's ordering and checks).
