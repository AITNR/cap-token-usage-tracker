//go:build !cgo || (!darwin && !linux)

package plugin

func loadedPluginPath() (string, bool) {
	return "", false
}
