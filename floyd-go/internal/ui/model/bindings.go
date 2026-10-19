package model

// AcceptSuggestionPrimaryBinding is the keybinding used to accept ghost-text suggestions.
// It must be a modified (non-printable-alone) key: a bare printable character
// would leak into the prompt text whenever no suggestion is present.
// The consistency lock in superfloyd_resilience.go verifies this has not drifted.
const AcceptSuggestionPrimaryBinding = "ctrl+y"
