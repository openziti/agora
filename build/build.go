package build

import "fmt"

// these are populated at release time via goreleaser ldflags
// (see the .goreleaser-*.yml configs); they stay empty for developer builds.
var (
	Version string
	Hash    string
	Date    string
	BuiltBy string
)

// Series is the current minor series for developer builds that carry no
// release version.
const Series = "v0.1"

// String returns a human-readable build identifier.
func String() string {
	if Version != "" {
		return fmt.Sprintf("%v [%v]", Version, Hash)
	}
	return Series + ".x [developer build]"
}
