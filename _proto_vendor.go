//go:build ignore

// Package quickgo includes proto files for proper vendoring
package quickgo

import (
	_ "embed"
)

// Embed proto files to ensure they are included in vendor
//
//go:embed grpcep/lib.proto
var _protoFile []byte