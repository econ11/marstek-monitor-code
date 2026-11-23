package app

/*
#cgo LDFLAGS: -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>

// firstPreferredLanguage writes the user's first preferred language
// ("uk-UA") into buf; 0 when there is none.
static int firstPreferredLanguage(char *buf, int n) {
	CFArrayRef langs = CFLocaleCopyPreferredLanguages();
	if (langs == NULL) {
		return 0;
	}
	int ok = 0;
	if (CFArrayGetCount(langs) > 0) {
		CFStringRef s = (CFStringRef)CFArrayGetValueAtIndex(langs, 0);
		ok = CFStringGetCString(s, buf, n, kCFStringEncodingUTF8);
	}
	CFRelease(langs);
	return ok;
}
*/
import "C"

import "unsafe"

// SystemLanguage is the first macOS preferred language ("uk-UA"), "" if
// unknown.
func SystemLanguage() string {
	buf := make([]byte, 64)
	if C.firstPreferredLanguage((*C.char)(unsafe.Pointer(&buf[0])), C.int(len(buf))) == 0 {
		return ""
	}
	return C.GoString((*C.char)(unsafe.Pointer(&buf[0])))
}
