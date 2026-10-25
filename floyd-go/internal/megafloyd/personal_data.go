package megafloyd

import "path/filepath"

// PersonalDataDir uses the already resolved workspace store when supplied.
// A canonical caller with only a cwd resolves the current account's private
// registry; it never discovers personal state in a shared project folder.
func PersonalDataDir(workingDir, selected string) (string, error) {
	if selected != "" {
		return filepath.Clean(selected), nil
	}
	if IsActive() {
		return WorkspaceDataDir(workingDir)
	}
	return filepath.Join(workingDir, ".floyd"), nil
}
