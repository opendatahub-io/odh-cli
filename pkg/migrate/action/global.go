package action

import "sync"

//nolint:gochecknoglobals // Global registry required for auto-registration pattern
var (
	globalRegistry     *ActionRegistry
	globalRegistryOnce sync.Once
)

func GetGlobalRegistry() *ActionRegistry {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewActionRegistry()
	})

	return globalRegistry
}

func MustRegisterAction(action Action) {
	GetGlobalRegistry().MustRegister(action)
}
