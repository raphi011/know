//go:build !cgo

package record

import "errors"

var errNoCGO = errors.New("audio support requires CGO (rebuild with CGO_ENABLED=1)")
