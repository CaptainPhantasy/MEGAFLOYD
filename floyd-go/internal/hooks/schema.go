package hooks

import _ "embed"

//go:embed payload.schema.json
var payloadSchema []byte

// PayloadSchema returns the versioned stdin contract without exposing mutable
// package storage. ValidatePayload also rejects ambiguous duplicate JSON keys.
func PayloadSchema() []byte { return append([]byte(nil), payloadSchema...) }
