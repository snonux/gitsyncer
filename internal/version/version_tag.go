package version

import "regexp"

var versionTagPattern = regexp.MustCompile(`^v?\d+(\.\d+)?(\.\d+)?$`)

// IsVersionTag checks if a tag name is a version tag.
// Supported formats: vX.Y.Z, vX.Y, vX, X.Y.Z, X.Y, X.
func IsVersionTag(tag string) bool {
	return versionTagPattern.MatchString(tag)
}
