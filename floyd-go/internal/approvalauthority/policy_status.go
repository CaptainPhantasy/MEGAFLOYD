package approvalauthority

// A deferred self-query is not proof of absent privilege and never authorizes
// a record. The separately privileged authority must verify account policy.
type localPolicyStatus uint8

const (
	localPolicyDeferred localPolicyStatus = iota
	localPolicyAbsent
)
