package contextx

import "github.com/gin-gonic/gin"

type contextKey string

const UserContextKey contextKey = "login_user_id"

type LoginUser struct {
	UserID   string
	Username string
	Role     string
	Avatar   string
}

func Set(c *gin.Context, u *LoginUser) {
	if c == nil || u == nil {
		return
	}
	c.Set(string(UserContextKey), u)
}

func Get(c *gin.Context) *LoginUser {
	if c == nil {
		return nil
	}
	if v, ok := c.Get(string(UserContextKey)); ok {
		if user, ok := v.(*LoginUser); ok {
			return user
		}
	}
	return nil
}

func Clear(c *gin.Context) {
	if c == nil || c.Keys == nil {
		return
	}
	delete(c.Keys, string(UserContextKey))
}
