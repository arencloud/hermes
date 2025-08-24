package auth

import (
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const sessionUserKey = "username"

func SessionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := sessions.Default(c)
		if v := sess.Get(sessionUserKey); v != nil {
			if username, ok := v.(string); ok {
				if u, ok := defaultStore.Get(username); ok {
					c.Set("user", u)
				}
			}
		}
		c.Next()
	}
}

func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get("user"); !exists {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Next()
	}
}

func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		v, exists := c.Get("user")
		if !exists {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		u := v.(*User)
		if u.Role != "admin" {
			c.String(http.StatusForbidden, "forbidden")
			c.Abort()
			return
		}
		c.Next()
	}
}

// Helpers for session login/logout
func Login(c *gin.Context, username string) {
	sess := sessions.Default(c)
	sess.Set(sessionUserKey, username)
	_ = sess.Save()
}

func Logout(c *gin.Context) {
	sess := sessions.Default(c)
	sess.Delete(sessionUserKey)
	_ = sess.Save()
}

// Global default store to simplify wiring in small app
var defaultStore = NewStore()

func GetStore() *Store { return defaultStore }
