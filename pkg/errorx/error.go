package errorx

type Error struct {
    Code  int    `json:"code"`
    Msg   string `json:"msg"`
    Cause error  `json:"-"` // 不序列化到 JSON
}

func (e *Error) Error() string {
    return e.Msg
}

func (e *Error) Unwrap() error {
    return e.Cause
}

func (e *Error) Is(target error) bool {
    t, ok := target.(*Error)
    if !ok {
        return false
    }
    return e.Code == t.Code
}

func (e *Error) Wrap(err error) *Error {
    if err == nil {
        return e
    }
    return &Error{
        Code:  e.Code,
        Msg:   e.Msg,
        Cause: err,
    }
}

func New(code int, msg string) *Error {
    return &Error{Code: code, Msg: msg}
}