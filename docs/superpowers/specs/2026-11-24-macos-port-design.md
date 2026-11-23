# macOS port, signed releases built on GitHub — design

Date: 2026-11-24 · Owner decisions taken in conversation · Size: project

## Intent and why

Marstek Monitor 1.0.0 is published for Windows. The owner wants a macOS version **for other people with a
Mac who download it from the releases page**, not only for themself. It should feel as finished as the
Windows one:
- it opens with a double click;
- it finds the station;
- it shows the same tabs;
- it warns about outages;
- it updates with one click.

A spike on the owner's Mac (Apple Silicon, macOS 26) proved that the app builds and runs, that the
menu-bar icon works and that it finds the real station. That spike lives in the private archive
repo (branch `econ11/macos-spike`, old module path, report `docs/macos-spike.md`). This design
turns it into a shipped product in the public code repo `econ11/marstek-monitor-code`.

## Goals

1. **The Mac app, at parity with Windows.** Everything the spike made real comes in, adapted to the
   current code (plan 3 added the update check and one-click update, the platform flag and
   journal link drops):
   - single instance (flock plus a Unix socket that says "show the window");
   - system language;
   - login item;
   - notifications through UNUserNotificationCenter;
   - sleep and wake observers;
   - the Local Network permission diagnosis instead of the firewall;
   - «Показати у теці» via `open -R`;
   - the notification settings URL;
   - the data folder `~/Library/Application Support/MarstekMonitor`;
   - the menu-bar template icon: filled bolt = on, outline = off, 16 pt, the `iconForGrid` rule.
2. **Dock icon (owner decision (a)).** The app is a menu-bar app. The Dock icon appears only while the
   window is open and goes away when it is closed (accessory ↔ regular activation policy). While
   visible, the Dock icon shows the grid state on/off like the Windows taskbar icon, using the same
   `iconForGrid` rule and the existing app icon on/off artwork.
3. **Mac wording.** Platform-specific i18n keys, chosen by the platform flag (`SettingsView.Platform`):
   - «Запускати під час входу в систему» / "Open at login", not «…разом з Windows»;
   - «Згортати в рядок меню», not «…в трей»;
   - the Local Network permission instead of the firewall and UAC;
   - the first-run hint about hidden tray icons dropped on Mac;
   - «як у системі» instead of «як у Windows».

   Uk and en go in both catalogues and in the copy doc.
4. **Notification click.** Clicking a notification opens the window on the matching tab, as on Windows.
   The spike did not verify this, so the Mac work must make sure it works.
5. **Signed and notarized distribution (owner decision).**
   - The app is signed with the owner's Apple **Developer ID Application** certificate, with hardened
     runtime, notarized by Apple and stapled.
   - It is shipped as `marstek-monitor-<version>-macos.dmg` (drag to Applications) plus
     `marstek-monitor-<version>-macos.dmg.sha256`.
   - It is one universal binary (arm64 + x86_64) for **macOS 12 or later**.
   - The owner accepts that macOS shows the developer's legal name from the certificate.
   - Entitlements: only what the app needs (network client; no sandbox unless Wails requires it).
   - `Info.plist` keeps `NSLocalNetworkUsageDescription` (uk/en via `InfoPlist.strings`) and the bundle
     id the spike used. Notification and Local Network permissions are tied to the bundle id and the
     signature, so they survive updates.
6. **One-click update on Mac**, with the same flow and UI as Windows (plan 3). The steps:
   1. The daily check and «Перевірити зараз» read `releases/latest`.
   2. «Оновити» downloads the `.dmg` of that release and verifies its SHA-256 against the `.sha256`
      asset.
   3. The database is backed up as on Windows (`backups/`, keep 3).
   4. The app mounts the dmg read-only (`hdiutil attach -nobrowse -readonly`), copies the `.app` to a
      temp folder on the same volume as the running app, and runs `codesign --verify --deep --strict`.
   5. It checks that the new app's **Team ID equals the running app's Team ID**.
   6. It swaps the bundles (rename, with rollback on failure), detaches the dmg, relaunches the new
      version and quits.

   If anything before the swap fails, nothing changes and the error is shown. If the app cannot
   replace itself, «Оновити» is not offered, only «Завантажити» (opens the release page). That covers
   an app that is not writable, is translocated by Gatekeeper, or runs from the dmg.

   Asset names are pinned like the Windows ones: `Latest` and `Restore` accept only the current
   platform's asset names under `ReleasesBase`. A release without a Mac asset never offers an update
   on Mac, and the same holds for Windows.
7. **Releases built on GitHub (owner decision A).**
   - **Trigger.** A tag `v<version>` pushed to the code repo runs one GitHub Actions workflow.
   - **Windows job** on a Windows runner:
     1. vet, test, npm test and typecheck;
     2. `wails3 build` with VERSION;
     3. NSIS;
     4. SHA-256.
   - **macOS job** on a macOS runner:
     1. the same checks;
     2. a universal build;
     3. sign, notarize and staple the app;
     4. build the dmg, then sign, notarize and staple it;
     5. SHA-256.
   - **Publish job.** It runs after both jobs pass and creates a **draft** release in
     `econ11/marstek-monitor` with all four assets and the notes from the CHANGELOG section of that
     version. The owner reviews the draft and publishes it by hand, so the publishing gate stays.
   - **CI on pushes and PRs.** The same test steps also run on both platforms for every push and PR,
     without building releases.
   - **Secrets** live in the code repo's Actions secrets, never in the tree:
     - the Developer ID certificate (`.p12` + password);
     - an App Store Connect API key for `notarytool` (key id, issuer id, `.p8`);
     - a fine-grained token with `contents: write` on `econ11/marstek-monitor` only.
   - `scripts/release.ps1` stays as a documented fallback for a Windows-only release from the owner's PC.
8. **Version 1.1.0** is the first release with both platforms (CHANGELOG entry for both).

## Non-goals

- Mac App Store and sandboxing for the store.
- Windows code signing (separate certificate, later).
- Multi-station UI.
- Sparkle or any third-party updater.
- Background or silent updates without a click.
- Linux.
- Any new station RPC (never `Bat.GetStatus`).

## Anti-goals

- An update replaces the app with a bundle not signed by the owner's Team ID, or not matching the
  published checksum.
- A failed update leaves the user with no working app.
- Secrets or the certificate end up in the public tree, CI logs or artifacts.
- Personal data in the public repo. This covers the owner's station LAN IP, serials, local paths and
  the e-mail. The spike report and screenshots contain the real IP and owner-specific notes, so they are
  **not** copied; only the code is ported.
- The Windows app changes behaviour. Windows users of 1.0.0 must update to 1.1.0 exactly as plan 3
  designed.
- A release appears with only one platform's assets.

## Constraints

- Go 1.26, Wails v3.1.0, the existing Taskfiles; cgo on macOS (spike files). Shared code in `core/`
  and `frontend/`; Windows code in `*_windows.go`, Mac code in `*_darwin.go`; one branch, `main`
  (plan 3 rule).
- The module path is `github.com/econ11/marstek-monitor-code`. The spike code uses the old path and
  is ported as a patch, not merged.
- macOS code can't be compiled with cgo from the Windows PC. Darwin work is built and tested on the
  owner's Mac (a Claude session there, as in the spike) and on the GitHub macOS runner. Windows work
  is tested here. `GOOS=windows go vet` must stay clean on the Mac and vice versa where possible.
- Tests use fakes: nothing calls GitHub, Apple or the station in tests. The update swap is tested
  with temp folders and fake bundles. The codesign and Team ID check sits behind an interface, so a
  fake verifier covers it.
- Never touch the owner's running installed app or its data folder on either machine. Run dev builds
  only with `-simulate*` and a temp data dir.
- The 3 `core/` tests rewritten in the spike for macOS loopback (`lantest.OtherHost`) come along.
- Copy rules as before: uk/en, keys in both catalogues, copy doc updated.

## Approaches

- **Signing:** (A) unsigned (free; Gatekeeper «Все одно відкрити» on first run, permissions probably
  re-asked after each update). (B) Developer ID + notarization, $99/year. **The owner chose B.**
- **Where releases are built:** (A) GitHub Actions for both platforms; one draft release with all
  assets; the certificate is held in secrets. (B) each platform built locally on its own machine; both
  machines are needed per release, and it is easy to ship one platform only. (C) a hybrid of the two,
  with no benefit. **The owner chose A.**
- **One separate releases repo for Mac** was considered and dropped by the owner. It would mean two
  links, two changelogs and versions to keep in sync. One releases repo with both platforms was chosen.
- **Mac updater:** (A) reuse plan 3's updater with a Mac installer step: dmg, codesign/Team ID check,
  bundle swap. (B) Sparkle, which needs an extra framework and an appcast feed and duplicates what
  exists. **Controller's call: A** (the owner accepted the summary).
- **Dock icon:** (a) only while the window is open, with grid state; (b) always; (c) never.
  **The owner chose (a).**

## Decided by the owner

- The Mac version is for other people downloading from releases.
- One releases repo for both platforms.
- Developer ID signing and notarization; the legal name in the signature is OK.
- Releases are built on GitHub Actions; draft plus a manual publish.
- The Dock icon is shown only while the window is open.
- The rest of the summary (own updater, 1.1.0 for both, universal, macOS 12+, data folder) is OK.

## Controller's calls (correctable)

- The asset names `…-macos.dmg` and `.sha256`.
- macOS 12 minimum.
- One universal binary.
- Mac updates use the dmg, not a separate zip.
- `release.ps1` kept as the Windows fallback.
- Darwin tasks are run in the owner's Mac session.
- Login item: keep the spike's LaunchAgent (works on 12+) unless signing makes `SMAppService` clearly
  better on 13+.

### Settled after the builder check (controller's calls, correctable)

- **Testing the Mac update end to end.**
  - Unit tests use fakes: temp bundles, a fake HTTP server and a fake signature verifier.
  - There is also one real run on the owner's Mac, with a dev-only override of the releases base that
    points at a local folder or server. The override exists only in builds without the `release` build
    tag; release builds keep `ReleasesBase` pinned, and a test asserts that.
  - The first real Mac update for users is 1.1.0 → the next release. A broken updater still leaves
    «Завантажити».
- **Support claims.** The README says "built for macOS 12+ on Apple Silicon and Intel; tested on Apple
  Silicon, macOS 26".
  - CI adds a cheap check on an Intel runner: both architectures are in the binary, and
    `codesign --verify` and `spctl --assess` pass.
  - There is no GUI test on CI.
- **Order of work while the enrolment is pending.**
  1. Port, Mac wording, Dock icon and the updater first, with ad-hoc signing.
  2. Then the workflow.
  3. Then the sign and notarize steps once the secrets exist.
  
  The tag-triggered release job **fails** if any signing secret is missing. It never publishes an
  unsigned Mac dmg.
- **Tags and releases.**
  - Pushing `v<semver>` (no `-pre`/`-rc`; a test rejects them) to the code repo runs the workflow.
  - The workflow creates a **draft** release `v<version>` in the releases repo, target `main`. The tag
    in the releases repo appears when the owner publishes the draft.
  - On a re-run, an existing draft for the same tag is deleted and recreated. An existing published
    release makes the job fail.
  - `release.ps1` is an emergency-only fallback, Windows only. For such a version, Mac users simply
    get no update offer; the docs say so.
- **No Team ID.** A running app with no Team ID (a dev or ad-hoc build) offers «Завантажити» only.
  The Team ID is read at runtime from the running bundle's own signature, never from a constant in the
  code.
- **Version stamping.** `CFBundleShortVersionString` and `CFBundleVersion` are templated from the
  build's VERSION (Taskfile var). Dev builds get `0.0.0-dev` and are treated as dev (never newer, never
  check).

## Owner actions (outside the code, each one owner-gated)

1. **Enrol in the Apple Developer Program** (individual, $99/year). Identity checks can take 1–2 days,
   so start now.
2. **Create the certificate.** Create the Developer ID Application certificate on the Mac and export
   it as `.p12` with a password.
3. **Create the App Store Connect API key** for notarization (Developer role).
4. **Create the fine-grained GitHub token** for the releases repo.
5. **Add the secrets** to the code repo (the agent prepares the exact list; the owner pastes the values).
6. **Acceptance.** On the owner's Mac: install from the published 1.1.0 dmg. On Windows: the 1.0.0 →
   1.1.0 one-click update.

## Left to the builder

- Wails specifics for the activation policy, the Dock icon image and the template tray icon.
- The notification click routing on macOS.
- The exact Info.plist and entitlements content.
- The dmg layout (an Applications symlink is enough).
- Workflow structure: reusable steps, caching, the runner images.
- How the relaunch after the swap is done (a detached helper waiting for the old pid).
- How the "can I replace myself" check works: writable parent folder; not translocated (the path is
  not under `/private/var/folders/.../AppTranslocation`).
