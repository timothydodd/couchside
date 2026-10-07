//go:build !windows

package main

func isService() bool { return false }

func serviceMain() {}
