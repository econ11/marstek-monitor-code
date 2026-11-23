package main

import "os"

// userDataBase is ~/Library/Application Support (not Caches: macOS may clear
// that folder, and it holds the history).
func userDataBase() (string, error) { return os.UserConfigDir() }
