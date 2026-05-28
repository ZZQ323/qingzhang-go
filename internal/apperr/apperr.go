// Package apperr 统一业务错误：携带业务码 + 用户可读消息。
// 仿 SZTU-iCampus-backend common/Exception 思路——业务层抛带码的错误，
// 出口层(handler.writeErr)统一翻译成 {code,msg} 响应，前端按 code/msg 反馈。
package apperr

import "fmt"

// 业务错误码（0 成功，其余各有含义，便于前端区分处理）
const (
	CodeInternal     = 1000 // 服务器内部错误（未归类）
	CodeParam        = 1001 // 参数错误
	CodeWxLogin      = 1002 // 微信登录换 openid 失败
	CodeUnauthorized = 1003 // 未登录 / token 失效
	CodeInvite       = 1004 // 邀请码无效
)

type Error struct {
	Code int
	Msg  string
}

func (e *Error) Error() string { return fmt.Sprintf("[%d] %s", e.Code, e.Msg) }

func New(code int, msg string) *Error { return &Error{Code: code, Msg: msg} }

// 便捷构造，语义清晰
func Param(msg string) *Error    { return New(CodeParam, msg) }
func WxLogin(msg string) *Error  { return New(CodeWxLogin, msg) }
func Invite(msg string) *Error   { return New(CodeInvite, msg) }
func Internal(msg string) *Error { return New(CodeInternal, msg) }
