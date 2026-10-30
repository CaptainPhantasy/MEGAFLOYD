//go:build linux

package approvalauthority

// Linux POSIX ACL rights are covered by the credential-dropped access probe.
func validateNativeACL([]pathAccess) error { return nil }
