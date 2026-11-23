package main

import "os"

// userDataBase is %LOCALAPPDATA%.
func userDataBase() (string, error) { return os.UserCacheDir() }
