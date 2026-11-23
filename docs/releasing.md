# Releasing

Publishing is the owner's decision. An agent may run the script only with `-DryRun`.

## Prerequisites

- Clean working tree on the release branch (default `main`, the release is made from main after the merge, override with `-Branch`).
- `CHANGELOG.md` has a non-empty `## <version>` section; it becomes the release notes.
- Go, Node, `wails3` and NSIS 3 installed. `makensis` is found on PATH or in `C:\Program Files (x86)\NSIS`; otherwise the script stops with a message.
- `gh` authenticated (`gh auth status`) with write access to `econ11/marstek-monitor`.

## Dry run

```powershell
scripts/release.ps1 -Version 1.0.0 -DryRun
```

It does everything except create the tag and the GitHub release, which are only printed:

1. clean tree on the release branch;
2. `v<version>` is not tagged yet;
3. CHANGELOG has `## <version>`;
4. `wails3 build`: installs the frontend dependencies and generates `frontend/bindings` and `frontend/dist` (all gitignored, so a fresh clone needs this before `go vet` and the typecheck);
5. `go vet ./...`, `go test ./...`, `npm --prefix frontend test`, `npm --prefix frontend run typecheck`;
6. `wails3 task package VERSION=<version>` produces `bin\marstek-monitor-<version>-setup.exe`;
7. SHA-256 goes to `bin\marstek-monitor-<version>-setup.exe.sha256` as `<hex>  <filename>` (this is what the in-app updater reads); the notes go to `bin\release-notes-<version>.md`;
8. re-check that the build left no tracked file changed (`git diff --quiet HEAD`, content-based: with `core.autocrlf=true` a same-content rewrite such as `go mod tidy` in a fresh clone is not a change);
9. `gh release create v<version> --repo econ11/marstek-monitor --title "Marstek Monitor <version>" --notes-file ... <installer> <installer>.sha256` (a `-rc`/`-pre` version adds `--prerelease`); only after it succeeds the local annotated tag `v<version>` is created (dry run: printed only).

Any failure stops the script. Asset names must stay exactly as above: the updater (Tasks 4/5) depends on them.

## The approval gate

Without `-DryRun` the script runs steps 1-8, then asks you to type `publish v<version>`. Anything else aborts and no tag is created; a failed `gh` leaves no tag either. After a successful publish it creates the local tag. The release repo `econ11/marstek-monitor` makes its own tag. Optionally, for traceability, push the code-repo tag yourself afterwards with `git push origin v<version>`; the script never pushes.

```powershell
scripts/release.ps1 -Version 1.0.0
```

After publishing:

1. **Owner's PC (first release only):** a hand-copied dev build never checks for updates, so move to the installed app by hand. Quit the running hand-copied app (tray → «Вийти»), run the published `marstek-monitor-<version>-setup.exe` interactively with the default folder (`%LOCALAPPDATA%\Programs\Marstek Monitor`, not the hand-copy folder), start it, and check that «Про програму» shows `<version>` and that HKCU Run `MarstekMonitor` points to the installed exe. Then the hand copy can be deleted. Firewall: the installed exe has a new path, so Windows may ask for network access again; allow it on **private** networks, or the app won't find the station (the app's own diagnosis offers the rule otherwise).
2. Install the published setup on a clean machine and check that "Check for updates" sees it.

## Tests

`Invoke-Pester scripts/release.Tests.ps1` checks the changelog section parser.
