//go:build !windows

package loom

func enableVTOutput() {} // no-op on Unix; VT sequences work natively
