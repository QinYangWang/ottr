//go:build !windows

package capture

// ddaAvailable is false off Windows: there is no Desktop Duplication API.
func ddaAvailable() bool { return false }
