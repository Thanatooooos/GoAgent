package errorcode

// IErrorCode 定义错误码接口，对应 Java 的 IErrorCode 接口
type IErrorCode interface {
	Code() string
	Message() string
}

// BaseErrorCode 定义错误码结构体
// 对应 Java 的 BaseErrorCode 类
type BaseErrorCode struct {
	code    string
	message string
}

// Code 实现接口方法，返回错误码
func (e BaseErrorCode) Code() string {
	return e.code
}

// Message 实现接口方法，返回错误信息
func (e BaseErrorCode) Message() string {
	return e.message
}

// Error 实现 error 接口，方便直接作为 error 返回
func (e BaseErrorCode) Error() string {
	return e.message
}

// ========== A 类错误：用户端错误 ==========

// 在 Go 中，我们直接实例化结构体变量
// 这种写法在 Go 中非常常见，类似于单例模式

// A000001 用户端错误
var ClientError = BaseErrorCode{code: "A000001", message: "用户端错误"}

// ========== A01 用户注册错误 ==========

// A000100 用户注册错误
var UserRegisterError = BaseErrorCode{code: "A000100", message: "用户注册错误"}

// A000110 用户名校验失败
var UserNameVerifyError = BaseErrorCode{code: "A000110", message: "用户名校验失败"}

// A000111 用户名已存在
var UserNameExistError = BaseErrorCode{code: "A000111", message: "用户名已存在"}

// A000112 用户名包含敏感词
var UserNameSensitiveError = BaseErrorCode{code: "A000112", message: "用户名包含敏感词"}

// A000113 用户名包含特殊字符
var UserNameSpecialCharacterError = BaseErrorCode{code: "A000113", message: "用户名包含特殊字符"}

// A000120 密码校验失败
var PasswordVerifyError = BaseErrorCode{code: "A000120", message: "密码校验失败"}

// A000121 密码长度不够
var PasswordShortError = BaseErrorCode{code: "A000121", message: "密码长度不够"}

// A000151 手机号格式校验失败
var PhoneVerifyError = BaseErrorCode{code: "A000151", message: "手机格式校验失败"}

// ========== A02 幂等性错误 ==========

// A000200 幂等 Token 为空
var IdempotentTokenNullError = BaseErrorCode{code: "A000200", message: "幂等Token为空"}

// A000201 幂等 Token 已被使用或失效
var IdempotentTokenDeleteError = BaseErrorCode{code: "A000201", message: "幂等Token已被使用或失效"}

// ========== A03 查询参数错误 ==========

// A000300 查询数据量超过最大限制
var SearchAmountExceedsLimit = BaseErrorCode{code: "A000300", message: "查询数据量超过最大限制"}

// ========== B 类错误：系统执行错误 ==========

// B000001 系统执行出错
var ServiceError = BaseErrorCode{code: "B000001", message: "系统执行出错"}

// B000100 系统执行超时
var ServiceTimeoutError = BaseErrorCode{code: "B000100", message: "系统执行超时"}

// ========== C 类错误：第三方服务错误 ==========

// C000001 调用第三方服务出错
var RemoteError = BaseErrorCode{code: "C000001", message: "调用第三方服务出错"}
