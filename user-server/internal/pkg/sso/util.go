// SSO 工具函数（2026-08-15 M3-P1-E4）
package sso

import (
	"crypto/elliptic"
	"crypto/rand"
	"io"
)

// curveFromName 从 JWK 曲线名获取椭圆曲线
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

// randRead 调用 crypto/rand 读取字节
func randRead(p []byte) (int, error) {
	return io.ReadFull(rand.Reader, p)
}

// readerFunc 把 func(p []byte) (int, error) 包装成 io.Reader
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


