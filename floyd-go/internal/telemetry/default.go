package telemetry

var defaultManager = NewManager()

func Default() *Manager {
	return defaultManager
}

func InitDefault(enabled bool, additional map[string]any) {
	defaultManager.Initialize(Config{
		Enabled:        enabled,
		AdditionalData: additional,
	})
}
