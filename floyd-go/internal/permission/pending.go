package permission

// Pending returns requests that another interface can still resolve.
func (s *permissionService) Pending() []PermissionRequest {
	s.activeRequestMu.Lock()
	defer s.activeRequestMu.Unlock()
	if s.activeRequest == nil {
		return []PermissionRequest{}
	}
	if _, ok := s.pendingRequests.Get(s.activeRequest.ID); !ok {
		return []PermissionRequest{}
	}
	return []PermissionRequest{*s.activeRequest}
}
