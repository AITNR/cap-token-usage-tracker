//go:build !cgo || (!darwin && !linux)

package main

func loadedPluginPath() (string, bool) {
	return "", false
}
