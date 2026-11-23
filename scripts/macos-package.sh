#!/bin/sh
# macos-package.sh <version> [<identity>]
#
# Builds the Mac release: a universal Marstek Monitor.app stamped with
# <version> (wails3 task darwin:package), then
#   bin/marstek-monitor-<version>-macos.dmg   the app + an Applications link
#   bin/marstek-monitor-<version>-macos.dmg.sha256   "<hex>  <name>"
#
# <identity>: a "Developer ID Application: …" signing identity in the
# keychain. Given, the app is signed with the hardened runtime, a secure
# timestamp and build/darwin/entitlements.plist, and so is the dmg. Without
# it everything stays ad-hoc signed (a test build: it cannot update itself).
#
# Notarization runs when NOTARY_KEY_ID, NOTARY_ISSUER and NOTARY_KEY_PATH (an
# App Store Connect API key, .p8) are all set: the app and then the dmg are
# submitted with notarytool and stapled. It needs an identity.
#
# UPDATE_TEST=1 in the environment builds the version WITHOUT the release tag
# (the local update test, docs/macos-signing.md); never for a release.
#
# Checks (any failure stops the script): the binary holds arm64 and x86_64;
# CFBundleShortVersionString is <version>; codesign --verify --deep --strict
# passes; with an identity, the hardened runtime flag is set.
set -eu

VERSION=${1:-}
IDENTITY=${2:-}
case "$VERSION" in
"" | *[!A-Za-z0-9._+-]*)
	echo "usage: $0 <version> [<identity>]; version: letters, digits and . _ + - only" >&2
	exit 2
	;;
esac

NOTARIZE=
if [ -n "${NOTARY_KEY_ID:-}" ] && [ -n "${NOTARY_ISSUER:-}" ] && [ -n "${NOTARY_KEY_PATH:-}" ]; then
	NOTARIZE=1
	if [ -z "$IDENTITY" ]; then
		echo "notarization needs a Developer ID identity" >&2
		exit 2
	fi
fi

cd "$(dirname "$0")/.."
BIN=bin
APP="$BIN/marstek-monitor.app"
EXE="$APP/Contents/MacOS/marstek-monitor"
DMG_NAME="marstek-monitor-$VERSION-macos.dmg"
DMG="$BIN/$DMG_NAME"

fail() {
	echo "macos-package: $*" >&2
	exit 1
}

notarize() { # <file to submit> <file to staple>
	xcrun notarytool submit "$1" --key "$NOTARY_KEY_PATH" --key-id "$NOTARY_KEY_ID" \
		--issuer "$NOTARY_ISSUER" --wait --timeout 30m >&2 || fail "notarization of $1 failed"
	xcrun stapler staple "$2" >&2 || fail "stapling $2 failed"
}

# 1. The universal, version-stamped, ad-hoc signed app.
wails3 task darwin:package VERSION="$VERSION" UPDATE_TEST="${UPDATE_TEST:-}"

# 2. Developer ID signature (hardened runtime, timestamp, entitlements).
if [ -n "$IDENTITY" ]; then
	codesign --force --options runtime --timestamp \
		--entitlements build/darwin/entitlements.plist \
		--sign "$IDENTITY" "$APP"
fi

# 3. Checks.
ARCHS=$(lipo -archs "$EXE")
case " $ARCHS " in *" arm64 "*) ;; *) fail "no arm64 in the binary ($ARCHS)" ;; esac
case " $ARCHS " in *" x86_64 "*) ;; *) fail "no x86_64 in the binary ($ARCHS)" ;; esac
SHORT=$(/usr/libexec/PlistBuddy -c "Print :CFBundleShortVersionString" "$APP/Contents/Info.plist")
[ "$SHORT" = "$VERSION" ] || fail "CFBundleShortVersionString is $SHORT, want $VERSION"
codesign --verify --deep --strict "$APP" || fail "codesign --verify --deep --strict failed"
if [ -n "$IDENTITY" ]; then
	codesign -dv "$APP" 2>&1 | grep -q 'flags=0x10000(runtime)' || fail "the hardened runtime is not set"
fi

# 4. Notarize and staple the app (before it goes into the dmg).
if [ -n "$NOTARIZE" ]; then
	ZIP=$(mktemp -d)/marstek-monitor.zip
	ditto -c -k --keepParent "$APP" "$ZIP"
	notarize "$ZIP" "$APP"
	rm -f "$ZIP"
fi

# 5. The dmg: the app (under its display name) and an Applications link.
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT
ditto "$APP" "$STAGE/Marstek Monitor.app"
ln -s /Applications "$STAGE/Applications"
rm -f "$DMG" "$DMG.sha256"
hdiutil create -quiet -volname "Marstek Monitor" -srcfolder "$STAGE" -fs HFS+ -format UDZO -ov "$DMG"
if [ -n "$IDENTITY" ]; then
	codesign --force --timestamp --sign "$IDENTITY" "$DMG"
fi
if [ -n "$NOTARIZE" ]; then
	notarize "$DMG" "$DMG"
fi
hdiutil verify -quiet "$DMG" || fail "hdiutil verify failed"

# 6. The checksum file, "<hex>  <name>" (core/update reads this format).
(cd "$BIN" && shasum -a 256 "$DMG_NAME" > "$DMG_NAME.sha256")

echo "$DMG"
echo "$DMG.sha256"
