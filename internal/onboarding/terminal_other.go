//go:build !windows

package onboarding

func enableVT(uintptr) (func(), error) { return func() {}, nil }
