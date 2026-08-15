package errors

// 业务错误码
const (
	CodeSuccess         = 0
	CodeParamInvalid    = 4000
	CodeUnauthorized    = 4001
	CodeForbidden       = 4003
	CodeNotFound        = 4004
	CodeConflict        = 4009
	CodeInternal        = 5000
	CodePlatformUnavail = 5001
	CodeAssetNotFound   = 6001
	CodeAssetDup        = 6002
	CodeAssetInvalid    = 6003
	CodeSyncFailed      = 6004
	CodeLoaderFallback  = 6005
)

// BizError 业务错误
type BizError struct {
	Code    int
	Message string
	Cause   error
}

func (e *BizError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *BizError) Unwrap() error { return e.Cause }

func New(code int, msg string) *BizError {
	return &BizError{Code: code, Message: msg}
}

func Wrap(code int, msg string, cause error) *BizError {
	return &BizError{Code: code, Message: msg, Cause: cause}
}

