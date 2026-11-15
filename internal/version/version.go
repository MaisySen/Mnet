// Package version holds the build version shared by every binary.
package version

// Version of this Mnet build.
const Version = "0.8.1"

// Revision identifies the source revision for deployment verification.
// Release/deployment builds set it with -ldflags -X.
var Revision = "development"
