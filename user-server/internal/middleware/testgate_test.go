package middleware

import "testing"

func init() {
	testModeGate = func() bool { return testing.Testing() }
}
