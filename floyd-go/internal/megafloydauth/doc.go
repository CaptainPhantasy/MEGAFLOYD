// Package megafloydauth holds the verified, non-network parts of the
// floyd-auth identity layer for megafloyd.
//
// The f7 harness (floyd7 / floyd-code TypeScript) authenticates against the
// owner's inference host with an RFC 8628 device-code flow. This package
// reproduces the pieces that could be verified from source and from the live
// home directory: environment resolution, credential slot naming, the
// on-disk token format, and the device identity headers.
//
// Status: design skeleton. Network calls are declared as interfaces and are
// deliberately not implemented. The endpoints are evidenced by the donor
// client and by method-404 responses from the live server, but no login has
// been exercised from Go. Callers must treat the flow as unimplemented until
// the evidence in docs/auth-integration.md is upgraded.
package megafloydauth
