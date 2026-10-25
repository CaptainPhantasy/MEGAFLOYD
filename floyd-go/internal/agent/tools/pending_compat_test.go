package tools

import "github.com/legacy-ai/floyd/internal/permission"

func (m *mockPermissionService) Pending() []permission.PermissionRequest      { return nil }
func (m *mockBashPermissionService) Pending() []permission.PermissionRequest  { return nil }
func (m *recordingPermissionService) Pending() []permission.PermissionRequest { return nil }
func (m *mockViewPermissionService) Pending() []permission.PermissionRequest  { return nil }
