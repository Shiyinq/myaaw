//go:build !darwin

package codesign

// Ensure is a no-op outside macOS.
func Ensure(path string) Result {
	_ = path
	return Result{}
}
