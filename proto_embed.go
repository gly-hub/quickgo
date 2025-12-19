// Package quickgo ensures proto files are included in vendor
package quickgo

import (
	_ "embed"
)

// This variable ensures the proto file is embedded in the binary,
// which forces Go to include it in vendor directories.
//
//go:embed grpcep/lib.proto
var libProto string