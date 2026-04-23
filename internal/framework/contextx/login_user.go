package contextx

import "github.com/gin-gonic/gin"

type contextKey string

const UserContextKey contextKey = "login_user_id"

// LoginUser 请求级别的登录用户信息（可扩展）
type LoginUser struct {
	UserId   string
	Username string
	Role     string
	Avatar   string
}

// Set 将登录用户信息写入 gin.Context
func Set(c *gin.Context, u *LoginUser) {
	if c == nil || u == nil {
		return
	}
	c.Set(string(UserContextKey), u)
}

// Get 从 gin.Context 读取登录用户信息，未设置返回 nil
func Get(c *gin.Context) *LoginUser {
	if c == nil {
		return nil
	}
	if v, ok := c.Get(string(UserContextKey)); ok {
		if lu, ok2 := v.(*LoginUser); ok2 {
			return lu
		}
	}
	return nil
}

// Clear 清理上下文中的登录用户信息
func Clear(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(string(UserContextKey), nil)
}
