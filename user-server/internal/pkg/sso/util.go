package sso

import (
	"crypto/elliptic"
	"crypto/rand"
	"io"
)

func curveFromName(name string) (elliptic.Curve, error) {
	switch name {
	case "P-256", "p-256":
		return elliptic.P256(), nil
	case "P-384", "p-384":
		return elliptic.P384(), nil
	case "P-521", "p-521":
		return elliptic.P521(), nil
	default:
		return nil, &UnsupportedCurveError{Name: name}
	}
}

// UnsupportedCurveError 不支持的椭圆曲线
type UnsupportedCurveError struct {
	Name string
}

func (e *UnsupportedCurveError) Error() string {
	return "unsupported curve: " + e.Name
}

func randRead(p []byte) (int, error) {
	return io.ReadFull(rand.Reader, p)
}

type readerFunc func(p []byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

// PKCES256 公开 PKCE S256（供外部调用）
func PKCES256(verifier string) string {
	return pkceS256(verifier)
}

// RandString 公开密码学安全随机字符串（供 controller 生成 state/nonce/verifier）
func RandString(n int) string {
	return randString(n)
}
