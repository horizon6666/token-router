package berror

const (
	CodeSuccess        = 0
	CodeAllocDuplicate = 20001 // info code: alloc hit idempotent path (HTTP 200)
	CodeInvalid        = 40001
	CodeNotFound       = 40401
	CodeOverloaded     = 42901
	CodeServerPanic    = 50001
)

type Error interface {
	error
	Code() int
	Msg() string
	Detail() string
	HTTPStatus() int
}

type bizError struct {
	code   int
	msg    string
	detail string
	status int
}

func (e *bizError) Error() string   { return e.msg }
func (e *bizError) Code() int       { return e.code }
func (e *bizError) Msg() string     { return e.msg }
func (e *bizError) Detail() string  { return e.detail }
func (e *bizError) HTTPStatus() int { return e.status }
func (e *bizError) WithDetail(s string) *bizError {
	cp := *e
	cp.detail = s
	return &cp
}

func New(code int, status int, msg string) *bizError {
	return &bizError{code: code, status: status, msg: msg}
}

var (
	ErrInvalid    = New(CodeInvalid, 400, "invalid_request")
	ErrNotFound   = New(CodeNotFound, 404, "not_found")
	ErrOverloaded = New(CodeOverloaded, 429, "overloaded")
	ErrPanic      = New(CodeServerPanic, 500, "internal_error")
)
