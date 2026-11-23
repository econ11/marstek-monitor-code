# macOS Port and Signed GitHub Releases Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship Marstek Monitor 1.1.0 for Windows and macOS from one GitHub Actions release, with a signed and notarized universal Mac app that updates itself with one click.

**Architecture:**
- The platform split from the macOS spike is ported into the public code repo: shared code in `core/`, `frontend/` and `app/*.go`, Windows in `*_windows.go`, Mac in `*_darwin.go` (+ `.m`).
- `core/update` learns per-platform asset names.
- The Mac one-click update is a platform-neutral swap routine with injected commands (testable anywhere), wired to `hdiutil`/`codesign`/`ditto` on darwin.
- One tag-triggered workflow builds both platforms and creates a draft release in the releases repo.

**Tech Stack:** Go 1.26 (cgo on darwin), Wails v3.1.0, TypeScript frontend (vitest), NSIS 3, Xcode command-line tools (`codesign`, `notarytool`, `stapler`, `hdiutil`), GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-11-24-macos-port-design.md`

**Spike source (read-only):** the private archive repo, branch `econ11/macos-spike` (head `fceef3d`, module path `github.com/econ11/marstek-app-windows`). The controller gives each implementer the patch they need; only code is ported, never `docs/macos-spike*`.

## Where each task runs

- **Mac:** needs cgo/darwin. It runs in the owner's Mac Claude session in a clone of `econ11/marstek-monitor-code`. The Windows controller hands it the task brief; the Mac session reports back.
- **Win:** runs here in `marstek-monitor-code-snapshot`.
- **Any:** platform-neutral Go or frontend, so it can run on either machine.

## Global Constraints

- Module `github.com/econ11/marstek-monitor-code`; one branch `main`; Windows code `*_windows.go`, Mac code `*_darwin.go`.
- The Windows app must not change behaviour. After every task, these pass on Windows: `go vet ./...`, `go test ./...`, `go test -race ./app/...`, `npm --prefix frontend test`, `npm --prefix frontend run typecheck` and `wails3 build`.
- After every Mac task these pass on the Mac: `go vet ./...`, `go test ./...`, `npm --prefix frontend test`, `GOOS=windows go vet ./...` and `wails3 package`.
- macOS 12 or later; one universal binary (arm64 + x86_64); bundle id unchanged from the spike's `build/darwin/Info.plist`.
- Data folder on Mac: `~/Library/Application Support/MarstekMonitor`.
- Mac release assets: `marstek-monitor-<v>-macos.dmg` and `marstek-monitor-<v>-macos.dmg.sha256` (`<hex>  <name>`). The Windows asset names are unchanged.
- The releases repo is pinned: `ReleasesBase` never changes in builds with the `release` build tag.
- A Mac update is accepted only if its SHA-256 matches, `codesign --verify --deep --strict` passes, and its Team ID equals the running app's Team ID, read at runtime. A running app with no Team ID offers «Завантажити» only.
- No personal data in the tree, CI logs or artifacts: no station IP, serials, local user paths or e-mail. Secrets live only in Actions secrets.
- Tests never call GitHub, Apple or the station. Never `Bat.GetStatus`.
- Never touch the owner's installed app or data folder on either machine. Dev runs use only `-simulate*` with a temp data dir.
- Copy: uk/en in both catalogues, new keys in the copy doc or under «not in the catalogue».
- Commits as `econ11 <14614307+econ11@users.noreply.github.com>`. Push only with owner OK.

## Review Focus

1. **The app runs from the dmg, from Downloads (translocated) or from a read-only folder.** «Оновити» must not be offered; «Завантажити» is. Owned by Task 6 (`TestCanSelfUpdate`).
2. **The swap fails half-way** (copy fails, rename of the new bundle fails). The old app must still be in place and launchable. Owned by Task 6 (`TestSwapRollsBack`).
3. **A 1.1.0 release carries both platforms' assets.** Windows must pick only the `.exe`, and Mac only the `.dmg`. A release missing the current platform's asset offers no install, and a Windows `.exe` URL stored in settings is dropped on Mac. Owned by Task 5 (`TestLatestPicksPlatformAsset`, `TestRestorePlatform`).
4. **The window closes through the red button, Cmd+W or Cmd+Q, with minimize-to-tray on and off.** The Dock icon goes away exactly when the window is hidden, and the app never keeps running with no Dock icon and no menu-bar icon. Owned by Task 3 (manual check list in Step 4 + `TestDockPolicy`).
5. **A Mac key exists in uk but not in en, or the reverse,** or a Windows-only text shows on Mac. Owned by Task 2 (`catalog.test.ts` extension + `TestNoWindowsWordOnMac` in `settingsModel.test.ts`).

---

### Task 1: Port the platform split from the spike (Mac)

**Files:**
- Create, from the spike, adapted to the current tree:
  - `app/appicon_windows.go`, `app/appicon_windows_test.go`, `app/appicon_darwin.go`
  - `app/instance_windows.go`, `app/instance_darwin.go`
  - `app/lang.go`, `app/lang_darwin.go`
  - `app/autostart_darwin.go`, `app/autostart_darwin_test.go`
  - `app/netdiag_darwin.go`, `app/netdiag_darwin_test.go`, `app/netdiag_windows_test.go`
  - `app/power_darwin.go`, `app/power_darwin.m`, `app/power_windows.go`
  - `app/settings_darwin.go`, `app/settings_windows.go`
  - `app/toast_darwin.go`, `app/toast_windows.go`, `app/toastplace_test.go`
  - `app/tray_darwin.go`, `app/tray_darwin_test.go`, `app/tray_windows.go`
  - `app/assets/menubar-{on,off}-*.png` (the spike head's 16 pt set)
  - `datadir_darwin.go`, `datadir_windows.go`
  - `build/darwin/{Info.plist,Info.dev.plist,Taskfile.yml,icons.icns}`
  - `docs/design/logo/menubar-{on,off}.svg`, `docs/design/logo/menubar_gen.go`
- Modify:
  - in `app/`: `appicon.go`, `lang_windows.go`, `power.go`, `settings.go`, `toast.go`, `toastpush.go`, `tray.go`, `window.go`, and the tests the spike moved;
  - `core/source/lan/lantest/fake.go` (`OtherHost`) and the 3 `core/` tests;
  - `main.go` (`userDataBase()`), `Taskfile.yml` (include `darwin`), `docs/design/logo/README.md`.
- Do not port: `docs/macos-spike.md`, `docs/macos-spike/`, the `BatteryDetails` notes.

**Interfaces:**
- Produces:
  - `func userDataBase() (string, error)`, per platform;
  - `func SystemLanguage() string`, per platform; `ResolveLanguage` is shared in `app/lang.go`;
  - `startInstaller` stays in `updater_windows.go`; `updater_other.go` stays the non-Windows stub until Task 6.
- Consumes: plan 3 code as it is. Plan 3 added `updater*.go`, `updates.go`, `autostart_windows.go` (`RepointAutostart`, `InstalledDir`) and `SettingsView.Platform`. Keep all of it; where the spike moved a function that plan 3 later changed, the current body wins and only moves.

- [ ] **Step 1:** Apply the spike patch.
  1. Generate it on the Mac with `git diff aa25a7c...origin/econ11/macos-spike -- . ':!docs/macos-spike*' ':!docs/macos-spike/**'` from the archive clone. The controller gives the exact base.
  2. Rewrite `github.com/econ11/marstek-app-windows` → `github.com/econ11/marstek-monitor-code`.
  3. Resolve conflicts by the rule in Interfaces above.
- [ ] **Step 2:** Grep the ported tree for the station IP, serials, `/Users/`, `C:\Users`, e-mail, `BatteryDetails`, `Bat.GetStatus` calls. Expected: none, except the existing «never call» comments.
- [ ] **Step 3:** Run the Mac checks from Global Constraints. Expected: all pass. `wails3 package` gives `bin/marstek-monitor.app` with an ad-hoc signature, and `codesign --verify --deep --strict` passes.
- [ ] **Step 4:** Run `bin/marstek-monitor.app` with `-simulate-quiet -simulate-dir $(mktemp -d)` through `open --args`. Check that the window, the menu-bar icon (on/off), a test notification and single instance (second launch shows the first) all work. Report with screenshots kept outside the repo.
- [ ] **Step 5:** On Windows, the controller runs the Windows checks on the pushed branch. Expected: all pass, and `wails3 build` gives an exe that behaves as 1.0.0.
- [ ] **Step 6: Commit** `feat(macos): platform split ported from the spike`.

---

### Task 2: Mac wording (Any)

**Files:**
- Modify: `frontend/src/i18n/uk.ts`, `frontend/src/i18n/en.ts`, `frontend/src/i18n/catalog.test.ts`, the copy doc (`docs/design/copy*.md` or wherever plan 2 keeps the catalogue).
- Create: `frontend/src/i18n/platform.ts`, `frontend/src/i18n/platform.test.ts`.
- Modify the call sites: `frontend/src/views/settingsModel.ts`, the first-run views (`firstRunModel.ts`, `firstRun.ts`) and the onboarding diagnosis view, plus their tests.

**Interfaces:**
- Produces:
  - `export type Platform = 'windows' | 'darwin'`;
  - `export function pk(key: string, platform: string): string`: returns `key + '.mac'` when `platform === 'darwin'` and that key exists in the uk catalogue, else `key`.
- Consumes: `SettingsView.Platform` (`runtime.GOOS`). The onboarding views get the platform the same way settings do; if they have no SettingsView, add `Platform` to the view they already receive from Go. One field, set from `runtime.GOOS`.

- [ ] **Step 1: Write failing tests**
  - In `platform.test.ts`:
    - `pk('settings.general.autostart','darwin') === 'settings.general.autostart.mac'`;
    - `pk(same,'windows') === 'settings.general.autostart'`;
    - `pk('no.such','darwin') === 'no.such'`.
  - In `catalog.test.ts`: every `*.mac` key exists in both uk and en, and its base key exists too.
  - In `settingsModel.test.ts` and the onboarding tests, add `TestNoWindowsWordOnMac`: with `Platform: 'darwin'`, no rendered string contains `Windows`, `трей`, `tray`, `брандмауер`, `firewall` or `UAC`.
- [ ] **Step 2:** `npm --prefix frontend test`. Expected: FAIL.
- [ ] **Step 3: Add the `.mac` keys and route the call sites through `pk`.**

  Exact uk copy:

  | Key | uk |
  |---|---|
  | `settings.general.autostart.mac` | «Відкривати під час входу в систему» |
  | `settings.general.minimizeToTray.mac` | «Згортати в рядок меню при закритті» |
  | `settings.general.language.desc.mac` | «За замовчуванням — мова macOS.» |
  | `settings.general.language.system.mac` | «Як у macOS» |
  | `settings.notify.test.desc.mac` | «Перевірити, що macOS показує сповіщення (наприклад, що їх не приховує режим зосередження).» |
  | `settings.notify.test.sent.mac` | «Надіслано. Не з'явилось? Відкрити налаштування сповіщень macOS» |
  | `settings.notify.osDisabled.mac` | «Сповіщення для Marstek Monitor вимкнено в macOS. Відкрити налаштування macOS» |
  | `settings.stations.trayShows.mac` | «У рядку меню показується: {station}» |
  | `help.localApi.firewall.mac` | «Якщо macOS запитає дозвіл на доступ до локальної мережі, натисніть «Дозволити».» |
  | `onboarding.dx.firewall.title.mac` | «Немає дозволу на локальну мережу» |
  | `onboarding.dx.firewall.body.mac` | «macOS не дозволяє Marstek Monitor шукати пристрої в локальній мережі. Увімкніть дозвіл у «Системних параметрах» → «Конфіденційність і безпека» → «Локальна мережа».» |
  | `onboarding.dx.firewall.action.mac` | «Відкрити налаштування» |

  The en keys mirror them: "Open at login", "Minimize to the menu bar when closed", "macOS language by default.", "Same as macOS", "Check that macOS shows notifications (for example, that a Focus mode doesn't hide them).", "Sent. Didn't see it? Open macOS notification settings", "Notifications for Marstek Monitor are turned off in macOS. Open macOS settings", "Shown in the menu bar: {station}", "If macOS asks for access to the local network, click "Allow".", "No local network permission", "macOS doesn't let Marstek Monitor look for devices on the local network. Turn it on in System Settings → Privacy & Security → Local Network.", "Open settings".

  On Mac, `onboarding.dx.netPublic.*` and `onboarding.done.trayHint` are not shown at all: `netdiag_darwin` never reports Public, and the hint is hidden when `platform === 'darwin'`. The `onboarding.dx.firewall.pending` and `.error` states never occur on Mac, because the action opens settings with no UAC.
- [ ] **Step 4:** `npm --prefix frontend test` and `npm --prefix frontend run typecheck`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(macos): Mac wording by platform`.

---

### Task 3: Dock icon only while the window is open, with grid state (Mac)

**Files:**
- Create: `app/dock_darwin.go`, `app/dock_darwin.m`, `app/dock.go`, `app/dock_test.go`.
- Modify: `app/window.go` (show/hide hooks), `app/appicon_darwin.go` (replace the stub).

**Interfaces:**
- Produces:
  - `type dockPolicy int` with values `dockHidden` and `dockShown`;
  - `func dockFor(windowVisible bool) dockPolicy`, pure, in `dock.go`;
  - `func setDock(p dockPolicy, icon appIcon)` (darwin; a no-op elsewhere): activation policy accessory/regular and `NSApp.applicationIconImage` from the on/off app icon PNG.
- Consumes: `iconForGrid(detect.GridState) appIcon`, the existing `Window.SetGrid` call path, and the app icon on/off artwork (`build/appicon.png` and its off variant; the controller points to the exact files).

- [ ] **Step 1: Write a failing test.** `TestDockPolicy`: `dockFor(true)==dockShown`, `dockFor(false)==dockHidden`. A grid change while hidden records the icon but does not show the Dock.
- [ ] **Step 2:** `go test ./app -run TestDockPolicy`. Expected: FAIL.
- [ ] **Step 3:** Implement. Window shown → regular policy plus the current icon. Window hidden (minimize-to-tray) → accessory. Quit → nothing to do. The menu-bar icon is always present, so the app is never invisible.
- [ ] **Step 4:** Manual check on the Mac with simulate. Cover: the red button and Cmd+W, with minimize-to-tray on and off; Cmd+Q; opening from the menu bar; grid off/on while the window is open (the Dock icon changes); grid change while hidden, then open (the right icon). Report the results.
- [ ] **Step 5: Commit** `feat(macos): Dock icon while the window is open, grid on/off`.

---

### Task 4: Notification click opens the matching tab (Mac)

**Files:**
- Modify: `app/toast_darwin.go`, `app/navigate.go` if needed.
- Test: `app/toast_darwin_test.go`.

**Interfaces:**
- Consumes: the Windows toast's navigation target (the same `navigate` target the Windows toast activation uses; see `app/navigate.go`); Wails `notifications` response callback.
- Produces: the darwin notification user info carries the navigate target under key `"nav"`; the response handler shows the window and navigates.

- [ ] **Step 1: Write a failing test.** `TestToastNavTarget`: a grid-lost toast carries `nav` = the outages target, a low-charge toast the overview target, and an update toast the About target. These are the same targets the Windows toast builder produces for the same events; compare against that function.
- [ ] **Step 2:** Run it. Expected: FAIL.
- [ ] **Step 3:** Implement the user info and the response handler.
- [ ] **Step 4:** Manual check on the Mac with simulate. Click a «Світло зникло» notification: the window opens on «Відключення». Repeat with the window closed and with it open.
- [ ] **Step 5: Commit** `feat(macos): notification click opens its tab`.

---

### Task 5: Per-platform release assets in `core/update` (Any)

**Files:**
- Modify: `core/update/update.go`, `core/update/update_test.go`, `app/updates.go`, `app/updater.go`.
- Create: `core/update/base_release.go` (`//go:build release`), `core/update/base_dev.go` (`//go:build !release`), `core/update/base_test.go`.

**Interfaces:**
- Produces:
  - `func AssetName(v, goos string) string`: `"windows"` → `"marstek-monitor-"+v+"-setup.exe"`, `"darwin"` → `"marstek-monitor-"+v+"-macos.dmg"`, anything else `""`.
  - `InstallerName(v)` is removed; callers use `AssetName(v, runtime.GOOS)`.
  - `Latest(ctx, c, current, goos string)` and `Restore(r Release, goos string)` pick and pin only that platform's asset and its `.sha256`.
  - `func releasesBase() string`. In `base_release.go` it returns the `ReleasesBase` constant. In `base_dev.go` it returns `MARSTEK_UPDATE_BASE` when that is set and is an `http://127.0.0.1:` or `https://` URL ending in `/`, else `ReleasesBase`. `LatestURL` and `pinned` use `releasesBase()`, and the API URL is derived the same way: `MARSTEK_UPDATE_API` in dev only.
- Consumes: the current `Latest`/`Restore`/`pinned` code.

- [ ] **Step 1: Write failing tests**
  - `TestLatestPicksPlatformAsset`: a release with all four assets gives the `.exe` URLs for `"windows"` and the `.dmg` URLs for `"darwin"`. A release with only the Windows assets gives `InstallerURL == ""` for `"darwin"`.
  - `TestRestorePlatform`: a stored release with the `.exe` URLs is dropped for `"darwin"` and kept for `"windows"`, and the reverse holds for the `.dmg`.
  - `TestAssetName`: the three cases above.
  - `TestReleaseBasePinned`, run with `go test -tags release ./core/update`: with `MARSTEK_UPDATE_BASE` set, `releasesBase() == ReleasesBase`.
  - `TestDevBaseOverride`: without the tag, a valid override is used, and `http://example.com/` and a base not ending in `/` are ignored.
- [ ] **Step 2:** `go test ./core/update/` and `go test -tags release ./core/update/`. Expected: FAIL.
- [ ] **Step 3:** Implement. In `app/`, pass `runtime.GOOS`. `install` uses `AssetName(rel.Version, runtime.GOOS)`. The `release` tag is added to the release builds in Task 7 (Mac) and Task 8 (Windows Taskfile/CI) via `EXTRA_TAGS`.
- [ ] **Step 4:** Run the Windows checks plus `go test -tags release ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(update): per-platform release assets; dev-only base override`.

---

### Task 6: Mac one-click update: verify, swap, relaunch (Any core + Mac wiring)

**Files:**
- Create:
  - `app/macupdate.go` (no build tag; pure logic with injected commands);
  - `app/macupdate_test.go`;
  - `app/updater_darwin.go` (real commands, `startInstaller`, `canSelfUpdate`).
- Modify:
  - `app/updater_other.go` becomes `//go:build !windows && !darwin`;
  - `app/updates.go`: `UpdateView` gets `CanInstall bool`;
  - `frontend/src/views/settingsModel.ts`: use `UpdateView.CanInstall` instead of `Platform === 'windows'`; Windows sets it to true;
  - their tests.

**Interfaces:**
- Produces:
  - In `macupdate.go`:

    ```go
    type macTools interface {
        Attach(dmg string) (mount string, err error)
        Detach(mount string) error
        Copy(src, dst string) error         // ditto
        Verify(app string) error            // codesign --verify --deep --strict
        TeamID(app string) (string, error)  // "" for ad-hoc/unsigned
    }
    func macInstall(t macTools, dmg, runningApp string) (newApp string, err error)
    func canSelfUpdate(t macTools, runningApp string, writable func(dir string) bool) bool
    ```

  - `macInstall` steps:
    1. It mounts the dmg and finds exactly one `*.app` at its root.
    2. It copies that app into `<parent of runningApp>/.marstek-update-<pid>/`.
    3. It runs Verify, then compares the TeamIDs: they must be equal and non-empty.
    4. It renames the running app to `<name>.old`, then renames the new one into place. If that second rename fails, it renames `.old` back.
    5. It detaches the dmg and removes the temp folder.
    6. It returns the path of the new app. `.old` is removed on the next start.
  - `canSelfUpdate` is false when any of these holds: the running app's TeamID is empty; the path contains `/AppTranslocation/`; the app is under `/Volumes/`; the parent folder is not writable.
  - `updater_darwin.go`: `startInstaller(dmg)` resolves the running `.app` from `os.Executable()`, calls `macInstall`, then starts a detached `/bin/sh -c 'while kill -0 <pid> 2>/dev/null; do sleep 0.2; done; open "<newApp>"'` and quits the app through the same path the Windows updater uses after `startInstaller`.
- Consumes: `install` in `app/updater.go`: download, checksum, backup and the restart marker unchanged. `AssetName` comes from Task 5.

- [ ] **Step 1: Write failing tests** with a fake `macTools` and temp folders:
  - `TestMacInstallSwaps`: the new bundle ends up in place, the old one in `.old`, the temp folder is gone, and Detach is called.
  - `TestMacInstallTeamMismatch`: the new TeamID differs from the running one, or is empty. Error, the running app is untouched, and no `.old` is created.
  - `TestMacInstallVerifyFails`: same outcome as the mismatch.
  - `TestMacInstallNoOrManyApps`: a dmg root with 0 or 2 apps gives an error.
  - `TestSwapRollsBack`: the second rename fails. The running path holds the original bundle again, unchanged.
  - `TestCanSelfUpdate`: a translocated path, `/Volumes/...`, an unwritable parent and an empty TeamID each give false; a normal `/Applications` app gives true.
  - `settingsModel.test.ts`: «Оновити» is enabled iff `update.CanInstall`.
- [ ] **Step 2:** `go test ./app -run 'TestMacInstall|TestSwap|TestCanSelfUpdate'` on Windows. Expected: FAIL.
- [ ] **Step 3:** Implement `macupdate.go` and the `CanInstall` plumbing (Any); then implement `updater_darwin.go` (Mac).
- [ ] **Step 4:** Run the Windows checks; on the Mac, the Mac checks.
- [ ] **Step 5: Real run on the Mac.** It uses the dev override from Task 5.
  1. Build two dev packages, 1.1.0-test1 and 1.1.0-test2, both signed with the same identity. Before the certificate exists, use an ad-hoc signature: then `canSelfUpdate` is false by design, so this step waits for Task 9's certificate. Use the Developer ID once it exists.
  2. Serve `-test2` and its `.sha256` from a local `python3 -m http.server` folder, with a fake `releases/latest` JSON.
  3. Run `-test1` from `~/Applications/MarstekTest/` with simulate and click «Оновити».
  4. Expected: test2 starts, «Про програму» shows test2, and the simulate data is intact.
- [ ] **Step 6: Commit** `feat(macos): one-click update swaps the signed app`.

---

### Task 7: Mac build: version stamp, universal binary, signed dmg (Mac)

**Files:**
- Modify: `build/darwin/Taskfile.yml`, `build/darwin/Info.plist`, `build/darwin/Info.dev.plist`.
- Create: `build/darwin/entitlements.plist`, `build/darwin/InfoPlist.strings` (uk, en), `scripts/macos-package.sh`.

**Interfaces:**
- Produces:
  - `task darwin:package VERSION=x.y.z` builds a universal `.app` with `-tags release`. `CFBundleShortVersionString` and `CFBundleVersion` are set from VERSION, with the same validation regex as Windows. `LSMinimumSystemVersion` is `12.0`.
  - `scripts/macos-package.sh <version> [<identity>]`:
    1. builds the dmg `bin/marstek-monitor-<v>-macos.dmg` (the app plus an Applications symlink, via `hdiutil create`) and its `.sha256`;
    2. signs with hardened runtime, `--timestamp` and the entitlements when an identity is given, and ad-hoc otherwise;
    3. when `NOTARY_KEY_ID`, `NOTARY_ISSUER` and `NOTARY_KEY_PATH` are set, notarizes and staples both the app and the dmg.
  - Without VERSION, the dev build is `0.0.0-dev` and is treated as dev.
- Consumes: the spike's `build/darwin` files from Task 1.

- [ ] **Step 1:** Add a check to `scripts/macos-package.sh` that fails on any of these: `lipo -archs` not listing both `arm64` and `x86_64`; `CFBundleShortVersionString` ≠ VERSION; `codesign --verify --deep --strict` failing; with an identity, `codesign -dv` lacking `flags=0x10000(runtime)`.
- [ ] **Step 2:** Run `scripts/macos-package.sh 1.1.0-test` without an identity. Expected: an ad-hoc dmg, and all checks pass.
- [ ] **Step 3:** Mount the dmg, drag the app to a temp Applications folder and launch it with simulate. It runs, and «Про програму» shows 1.1.0-test.
- [ ] **Step 4: Commit** `build(macos): versioned universal app and dmg; signing and notarization when configured`.

---

### Task 8: GitHub Actions: CI and the tag-triggered draft release (Any)

**Files:**
- Create: `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `scripts/release-notes.ps1` (the CHANGELOG section extraction moved out of `release.ps1`, so both share it).
- Modify: `scripts/release.ps1` (uses `release-notes.ps1`; header says emergency, Windows-only fallback), `build/windows/Taskfile.yml` (`-tags release` for VERSION builds).

**Interfaces:**
- **`ci.yml`:**
  - on push and pull_request, a matrix of `windows-latest` and `macos-latest`;
  - Go 1.26 and Node 22;
  - `go vet ./...`, `go test ./...`, `npm --prefix frontend ci`, `npm --prefix frontend test`, `npm --prefix frontend run typecheck`;
  - on macOS also `go test -tags release ./core/update/`.
- **`release.yml`:**
  - on push of tags `v[0-9]+.[0-9]+.[0-9]+`, plus `workflow_dispatch` with input `version` and `dry_run` (default true);
  - first step: reject a version with `-` or `+`.
  - **Job `windows`:** the checks, `wails3 build VERSION=<v> EXTRA_TAGS=release`, NSIS, SHA-256, then upload the artifacts.
  - **Job `macos`:**
    1. Fail at once if any secret is missing: `MACOS_CERT_P12`, `MACOS_CERT_PASSWORD`, `NOTARY_KEY_ID`, `NOTARY_ISSUER`, `NOTARY_KEY_P8`.
    2. Import the cert into a temporary keychain.
    3. Write the `.p8` to `$RUNNER_TEMP`.
    4. Run `scripts/macos-package.sh <v> "Developer ID Application"`.
    5. Run `spctl --assess --type execute` on the app.
    6. Upload the artifacts.
    7. Delete the keychain and the key in an `always()` step.
  - **Job `intel-check`** (`macos-13`, needs `macos`): download the dmg, check `lipo` and `spctl --assess`.
  - **Job `publish`** (needs all three; skipped when `dry_run`):
    1. Extract the notes with `scripts/release-notes.ps1`.
    2. If the releases repo has a published `v<v>`, fail. If it has a draft, delete it.
    3. `gh release create v<v> --repo econ11/marstek-monitor --draft --target main --title "Marstek Monitor <v>" --notes-file …` with the four assets, using `RELEASES_TOKEN`.
  - No step echoes a secret. `set -x` is never used in the signing steps.
- Consumes: Task 7's script, the Windows Taskfile, CHANGELOG.md.

- [ ] **Step 1:** Run `actionlint` locally on both workflows (`go run github.com/rhysd/actionlint/cmd/actionlint@latest`). Expected: no errors.
- [ ] **Step 2:** Push the branch (owner OK) and watch `ci.yml` on both runners. Expected: green.
- [ ] **Step 3:** Run `release.yml` through `workflow_dispatch` with `version=1.1.0` and `dry_run=true`. Until the secrets exist, the expected result is that the macOS job fails with the "missing secret" message, the Windows job is green, and nothing is published. After Task 9, expect all jobs green and publish skipped.
- [ ] **Step 4: Commit** `ci: tests on Windows and macOS; tag-triggered draft release with both platforms`.

---

### Task 9: Owner setup of signing secrets, docs and CHANGELOG (Owner + Any)

**Files:**
- Create: `docs/macos-signing.md`, the owner's steps with no values:
  1. the Developer ID Application certificate on the Mac;
  2. export it to `.p12`;
  3. create an App Store Connect API key (Developer role);
  4. create a fine-grained token `contents: write` on `econ11/marstek-monitor` only;
  5. `gh secret set` for each of the six names, from files, never typed into a chat.
- Modify:
  - `CHANGELOG.md`: a 1.1.0 entry, uk/en; the macOS version and GitHub-built releases;
  - `docs/releasing.md`: the tag → draft → publish flow, and `release.ps1` as the emergency fallback;
  - the releases repo `README.md` (its source is in the private archive's `release-repo/`; from now on edit the releases repo directly). Add a macOS section, uk/en: download the dmg, drag it to «Програми», the Local Network permission, «built for macOS 12+ on Apple Silicon and Intel; tested on Apple Silicon, macOS 26», updates.

- [ ] **Step 1:** Write the docs. Grep them for personal data. Expected: none.
- [ ] **Step 2:** The owner follows `docs/macos-signing.md`. The controller checks with `gh secret list` that the six names exist (the values stay unseen).
- [ ] **Step 3:** Rerun Task 6 Step 5 (the real swap with the Developer ID) and Task 8 Step 3 (dry run, all green).
- [ ] **Step 4: Commit** `docs: macOS signing setup, releasing, CHANGELOG 1.1.0`.

---

### Task 10: Release 1.1.0 and owner acceptance (Owner-gated)

- [ ] **Step 1:** With owner OK, push tag `v1.1.0` to the code repo. Expected: a draft `v1.1.0` in the releases repo with four assets.
- [ ] **Step 2:** The controller checks the draft:
  - the asset names;
  - the checksums match the artifacts;
  - the notes;
  - no personal data in the notes or the logs.
- [ ] **Step 1b:** Right after the draft is checked and before publishing, with owner OK: push the releases repo README (the macOS section, a local commit in the releases clone), so the published release links to a README that covers the Mac.
- [ ] **Step 3:** The owner publishes the draft by hand.
- [ ] **Step 4:** Acceptance checklist for the owner:
  - **Mac:**
    1. Download the dmg in Safari and open it. The first launch shows only macOS's usual one-time confirmation for an app downloaded from the Internet («Відкрити»); no other warning (no unidentified developer, no "cannot check it for malicious software").
    2. Drag the app to «Програми» and launch it.
    3. The Local Network prompt shows; allow it. The real station is found.
    4. Notifications work, and clicking one opens its tab.
    5. The Dock icon appears and disappears with the window.
    6. «Відкривати під час входу» survives a log out and back in.
    7. Sleep and wake are logged.
    8. «Про програму» shows 1.1.0, and «Перевірити зараз» says up to date.
  - **Windows:** the installed 1.0.0 offers 1.1.0, «Оновити» works, the history is kept, and «Про програму» shows 1.1.0.
- [ ] **Step 5:** Record the results in `docs/dev-notes.md` («Приймання macOS 1.1.0»). The first real Mac self-update is checked at the next release.
