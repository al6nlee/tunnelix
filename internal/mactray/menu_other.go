//go:build !darwin || !cgo || purego

package mactray

func Enable() error { return nil }
