package workspace

// ServerHost returns the actual backend hosting a workspace. A local workspace
// has no shared host and cannot hand a live agent to another interface.
func ServerHost(w Workspace) string {
	if client, ok := w.(*ClientWorkspace); ok {
		return client.client.ServerHost()
	}
	return ""
}
