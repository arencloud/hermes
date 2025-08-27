package routes

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/arencloud/hermes/internal/models"
)

// simpleAuthMiddleware checks for a cookie "auth"; if not present, redirects to /login.
func simpleAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, err := c.Cookie("auth"); err != nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Next()
	}
}

// injectUser places basic user context into the template based on cookies.
func injectUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		username, _ := c.Cookie("user")
		role, _ := c.Cookie("role")
		canPull, _ := c.Cookie("canPull")
		canPush, _ := c.Cookie("canPush")
		hasRole, _ := c.Cookie("hasRole")
		c.Set("user:name", username)
		c.Set("user:role", role)
		c.Set("user:canPull", canPull == "1")
		c.Set("user:canPush", canPush == "1")
		c.Set("user:hasRole", hasRole == "1")
		c.Next()
	}
}

func RegisterWebRoutes(app *gin.Engine, db *gorm.DB) {
	app.Use(injectUser())

	// Favicon: serve Hermes icon at /favicon.ico (SVG)
	app.GET("/favicon.ico", func(c *gin.Context) {
		// Same visual as the inline favicon used in base.html
		svg := `<?xml version="1.0" encoding="UTF-8"?>
		<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">
		  <rect width="64" height="64" rx="12" ry="12" fill="#000"/>
		  <path d="M36 6L14 36h12l-4 22 28-34H38l-2-18z" fill="#FFC107"/>
		</svg>`
		c.Data(http.StatusOK, "image/svg+xml", []byte(svg))
	})

	// Home -> shows login as main page if not authenticated; redirects to /dashboard if authenticated
	app.GET("/", func(c *gin.Context) {
		if _, err := c.Cookie("auth"); err != nil {
			// Not authenticated: render login as main page with orgs and message
			var orgs []models.Organization
			if err := db.Order("name asc").Find(&orgs).Error; err != nil {
				c.HTML(http.StatusInternalServerError, "login.html", gin.H{"Title": "Login", "Error": err.Error()})
				return
			}
			// handle optional query params for success message/prefill
			loginMsg := "Please sign in to continue"
			if c.Query("signup") == "1" {
				if orgName := c.Query("orgName"); orgName != "" {
					loginMsg = "Account created for '" + orgName + "'. Please sign in."
				} else {
					loginMsg = "Account created. Please sign in."
				}
			}
			var selectedOrgID uint
			if v := c.Query("orgId"); v != "" {
				if n, err := strconv.ParseUint(v, 10, 64); err == nil {
					selectedOrgID = uint(n)
				}
			}
			c.HTML(http.StatusOK, "login.html", gin.H{
				"Title":         "Login",
				"Organizations": orgs,
				"LoginMessage":  loginMsg,
				"SelectedOrgID": selectedOrgID,
				"PrefillEmail":  c.Query("email"),
			})
			return
		}
		// Authenticated: redirect to dashboard for a clear page change
		c.Redirect(http.StatusFound, "/dashboard")
	})

	// Login
	app.GET("/login", func(c *gin.Context) {
		// If already authenticated, go to dashboard
		if _, err := c.Cookie("auth"); err == nil {
			c.Redirect(http.StatusFound, "/dashboard")
			return
		}
		// Load organizations for dropdown
		var orgs []models.Organization
		if err := db.Order("name asc").Find(&orgs).Error; err != nil {
			c.HTML(http.StatusInternalServerError, "login.html", gin.H{"Title": "Login", "Error": err.Error()})
			return
		}
		// Read optional query params to improve UX after signup
		loginMsg := ""
		if c.Query("signup") == "1" {
			if orgName := c.Query("orgName"); orgName != "" {
				loginMsg = "Account created for '" + orgName + "'. Please sign in."
			} else {
				loginMsg = "Account created. Please sign in."
			}
		}
		var selectedOrgID uint
		if v := c.Query("orgId"); v != "" {
			if n, err := strconv.ParseUint(v, 10, 64); err == nil {
				selectedOrgID = uint(n)
			}
		}
		c.HTML(http.StatusOK, "login.html", gin.H{
			"Title":         "Login",
			"Organizations": orgs,
			"LoginMessage":  loginMsg,
			"SelectedOrgID": selectedOrgID,
			"PrefillEmail":  c.Query("email"),
		})
	})
	app.POST("/login", func(c *gin.Context) {
		orgIDStr := c.PostForm("orgId")
		email := c.PostForm("email")
		password := c.PostForm("password")
		if orgIDStr == "" || email == "" || password == "" {
			c.HTML(http.StatusBadRequest, "login.html", gin.H{"Title": "Login", "Error": "Organization, email and password are required"})
			return
		}
		// parse org id
		var org models.Organization
		if err := db.Where("id = ?", orgIDStr).First(&org).Error; err != nil {
			c.HTML(http.StatusUnauthorized, "login.html", gin.H{"Title": "Login", "Error": "Invalid organization or credentials"})
			return
		}
		var user models.User
		if err := db.Where("organization_id = ? AND email = ?", org.ID, email).First(&user).Error; err != nil {
			c.HTML(http.StatusUnauthorized, "login.html", gin.H{"Title": "Login", "Error": "Invalid organization or credentials"})
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
			c.HTML(http.StatusUnauthorized, "login.html", gin.H{"Title": "Login", "Error": "Invalid organization or credentials"})
			return
		}
		// determine role: admin if Super Admin and compute effective permissions (user roles + group roles)
		roleName := "user"
		canPull := false
		canPush := false
		hasRole := false
		// Check direct user role
		var userRoles []models.UserRole
		_ = db.Where("organization_id = ? AND user_id = ?", org.ID, user.ID).Find(&userRoles).Error
		for _, ur := range userRoles {
			hasRole = true
			var role models.Role
			if err := db.Where("organization_id = ? AND id = ?", org.ID, ur.RoleID).First(&role).Error; err == nil {
				if role.Key == "super_admin" || role.Name == "Super Admin" {
					roleName = "admin"
					// Super Admin must always have access to buckets management and list
					canPull = true
					canPush = true
				} else {
					if role.CanPull {
						canPull = true
					}
					if role.CanPush {
						canPush = true
					}
				}
			}
		}
		// Check group roles through memberships
		var memberships []models.UserGroup
		_ = db.Where("organization_id = ? AND user_id = ?", org.ID, user.ID).Find(&memberships).Error
		for _, m := range memberships {
			var grs []models.GroupRole
			if err := db.Where("organization_id = ? AND group_id = ?", org.ID, m.GroupID).Find(&grs).Error; err == nil {
				for _, gr := range grs {
					hasRole = true
					var role models.Role
					if err := db.Where("organization_id = ? AND id = ?", org.ID, gr.RoleID).First(&role).Error; err == nil {
 					if role.Key == "super_admin" || role.Name == "Super Admin" {
 						roleName = "admin"
 						// Super Admin must always have access to buckets management and list
 						canPull = true
 						canPush = true
 					} else {
 						if role.CanPull {
 							canPull = true
 						}
 						if role.CanPush {
 							canPush = true
 						}
 					}
					}
				}
			}
		}
		// Set cookies and redirect to dashboard
		exp := time.Now().Add(12 * time.Hour)
		ttl := int(exp.Sub(time.Now()).Seconds())
		c.SetCookie("auth", "1", ttl, "/", "", false, true)
		c.SetCookie("user", email, ttl, "/", "", false, true)
		c.SetCookie("role", roleName, ttl, "/", "", false, true)
		c.SetCookie("org", fmt.Sprint(org.ID), ttl, "/", "", false, true)
		c.SetCookie("orgName", org.Name, ttl, "/", "", false, true)
		c.SetCookie("canPull", map[bool]string{true: "1", false: "0"}[canPull], ttl, "/", "", false, true)
		c.SetCookie("canPush", map[bool]string{true: "1", false: "0"}[canPush], ttl, "/", "", false, true)
		c.SetCookie("hasRole", map[bool]string{true: "1", false: "0"}[hasRole], ttl, "/", "", false, true)
		c.Redirect(http.StatusFound, "/dashboard")
	})

	// Signup
	app.GET("/signup", func(c *gin.Context) {
		// If already authenticated, go to dashboard instead of showing signup
		if _, err := c.Cookie("auth"); err == nil {
			c.Redirect(http.StatusFound, "/dashboard")
			return
		}
		c.HTML(http.StatusOK, "signup.html", gin.H{"Title": "Sign up"})
	})
	app.POST("/signup", func(c *gin.Context) {
		orgName := c.PostForm("organizationName")
		email := c.PostForm("email")
		displayName := c.PostForm("displayName")
		password := c.PostForm("password")
		if orgName == "" || email == "" || password == "" {
			c.HTML(http.StatusBadRequest, "signup.html", gin.H{"Title": "Sign up", "Error": "Organization, email and password are required"})
			return
		}
		// Start a transaction
		tx := db.Begin()
		if tx.Error != nil {
			c.String(http.StatusInternalServerError, tx.Error.Error())
			return
		}
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()
		// Create Organization (generate unique slug)
		org := models.Organization{Name: orgName}
		// Basic slugify similar to API path
		slugBase := func(s string) string {
			out := make([]rune, 0, len(s))
			prevDash := false
			for _, r := range []rune(s) {
				if r >= 'A' && r <= 'Z' { r = r + 32 }
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
					out = append(out, r); prevDash = false; continue
				}
				if !prevDash { out = append(out, '-'); prevDash = true }
			}
			start, end := 0, len(out)
			for start < end && out[start] == '-' { start++ }
			for end > start && out[end-1] == '-' { end-- }
			if start >= end { return "org" }
			return string(out[start:end])
		}(orgName)
		// ensure uniqueness within transaction
		slug := slugBase
		if slug == "" { slug = "org" }
		var exists models.Organization
		idx := 1
		for {
			if err := tx.Where("slug = ?", slug).First(&exists).Error; err != nil {
				if err == gorm.ErrRecordNotFound { break }
				tx.Rollback(); c.String(http.StatusInternalServerError, err.Error()); return
			}
			idx++
			slug = fmt.Sprintf("%s-%d", slugBase, idx)
		}
		org.Slug = slug
		if err := tx.Create(&org).Error; err != nil {
			tx.Rollback()
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		// Ensure Super Admin role
		role := models.Role{OrganizationID: org.ID, Name: "Super Admin", Key: "super_admin", IsSystem: true, Description: "Has full access within the organization", CanPull: true, CanPush: true}
		if err := tx.Where("organization_id = ? AND name = ?", org.ID, "Super Admin").FirstOrCreate(&role).Error; err != nil {
			tx.Rollback()
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		// Create User with hashed password
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			tx.Rollback()
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		user := models.User{OrganizationID: org.ID, Email: email, DisplayName: displayName, PasswordHash: string(hash)}
		if err := tx.Create(&user).Error; err != nil {
			tx.Rollback()
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		// Assign Super Admin role to user
		if err := tx.Create(&models.UserRole{OrganizationID: org.ID, UserID: user.ID, RoleID: role.ID}).Error; err != nil {
			tx.Rollback()
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		if err := tx.Commit().Error; err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		// Redirect to login with success message and credential hints
		c.Redirect(http.StatusFound, "/login?signup=1&orgId="+fmt.Sprint(org.ID)+"&orgName="+url.QueryEscape(org.Name)+"&email="+url.QueryEscape(email))
	})

	// Logout
	app.GET("/logout", func(c *gin.Context) {
		c.SetCookie("auth", "", -1, "/", "", false, true)
		c.SetCookie("user", "", -1, "/", "", false, true)
		c.SetCookie("role", "", -1, "/", "", false, true)
		c.SetCookie("canPull", "", -1, "/", "", false, true)
		c.SetCookie("canPush", "", -1, "/", "", false, true)
		c.SetCookie("hasRole", "", -1, "/", "", false, true)
		c.Redirect(http.StatusFound, "/login")
	})

	// Dashboard (auth required)
	app.GET("/dashboard", simpleAuthMiddleware(), func(c *gin.Context) {
		username, _ := c.Get("user:name")
		role, _ := c.Get("user:role")
		c.HTML(http.StatusOK, "dashboard.html", gin.H{
			"Title":    "Dashboard",
			"Username": username,
			"Role":     role,
			"CanPull":  func() bool { v, _ := c.Get("user:canPull"); return v == true }(),
			"CanPush":  func() bool { v, _ := c.Get("user:canPush"); return v == true }(),
			"HasRole":  func() bool { v, _ := c.Get("user:hasRole"); return v == true }(),
		})
	})

	// Admin page (admin only)
	app.GET("/admin", simpleAuthMiddleware(), func(c *gin.Context) {
		roleVal, _ := c.Get("user:role")
		username, _ := c.Get("user:name")
		if roleVal != "admin" {
			c.HTML(http.StatusForbidden, "forbidden.html", gin.H{"Title": "Forbidden", "Username": username, "Role": roleVal})
			return
		}
		c.HTML(http.StatusOK, "admin.html", gin.H{"Title": "Administration", "Username": username, "Role": roleVal, "CanPull": func() bool { v, _ := c.Get("user:canPull"); return v == true }(), "CanPush": func() bool { v, _ := c.Get("user:canPush"); return v == true }(), "HasRole": func() bool { v, _ := c.Get("user:hasRole"); return v == true }()})
	})

	// Admin Users & Roles page (admin only)
	app.GET("/admin/users-roles", simpleAuthMiddleware(), func(c *gin.Context) {
		roleVal, _ := c.Get("user:role")
		username, _ := c.Get("user:name")
		if roleVal != "admin" {
			c.HTML(http.StatusForbidden, "forbidden.html", gin.H{"Title": "Forbidden", "Username": username, "Role": roleVal})
			return
		}
		orgID, _ := c.Cookie("org")
		orgName, _ := c.Cookie("orgName")
		c.HTML(http.StatusOK, "admin_users_roles.html", gin.H{
			"Title":    "Users & Roles",
			"Username": username,
			"Role":     roleVal,
			"OrgID":    orgID,
			"OrgName":  orgName,
			"CanPull":  func() bool { v, _ := c.Get("user:canPull"); return v == true }(),
			"CanPush":  func() bool { v, _ := c.Get("user:canPush"); return v == true }(),
			"HasRole":  func() bool { v, _ := c.Get("user:hasRole"); return v == true }(),
		})
	})

	// Buckets management page (requires push permission)
	app.GET("/buckets", simpleAuthMiddleware(), func(c *gin.Context) {
		username, _ := c.Get("user:name")
		roleVal, _ := c.Get("user:role")
		if ok, _ := c.Get("user:canPush"); ok != true {
			c.HTML(http.StatusForbidden, "forbidden.html", gin.H{"Title": "Forbidden", "Username": username, "Role": roleVal})
			return
		}
		orgID, _ := c.Cookie("org")
		orgName, _ := c.Cookie("orgName")
		c.HTML(http.StatusOK, "buckets_manage.html", gin.H{
			"Title":    "Buckets Management",
			"Username": username,
			"Role":     roleVal,
			"OrgID":    orgID,
			"OrgName":  orgName,
			"CanPush":  true,
			"CanPull":  true,
			"HasRole":  func() bool { v, _ := c.Get("user:hasRole"); return v == true }(),
		})
	})
	// Buckets list page (requires pull permission)
	app.GET("/buckets/list", simpleAuthMiddleware(), func(c *gin.Context) {
		username, _ := c.Get("user:name")
		roleVal, _ := c.Get("user:role")
		if ok, _ := c.Get("user:canPull"); ok != true {
			c.HTML(http.StatusForbidden, "forbidden.html", gin.H{"Title": "Forbidden", "Username": username, "Role": roleVal})
			return
		}
		orgID, _ := c.Cookie("org")
		orgName, _ := c.Cookie("orgName")
		c.HTML(http.StatusOK, "buckets_list.html", gin.H{
			"Title":    "Buckets List",
			"Username": username,
			"Role":     roleVal,
			"OrgID":    orgID,
			"OrgName":  orgName,
			"CanPull":  true,
			"CanPush":  func() bool { v, _ := c.Get("user:canPush"); return v == true }(),
			"HasRole":  func() bool { v, _ := c.Get("user:hasRole"); return v == true }(),
		})
	})

	// Browse objects page (requires pull permission)
	app.GET("/buckets/browse/:storageId", simpleAuthMiddleware(), func(c *gin.Context) {
		username, _ := c.Get("user:name")
		roleVal, _ := c.Get("user:role")
		if ok, _ := c.Get("user:canPull"); ok != true {
			c.HTML(http.StatusForbidden, "forbidden.html", gin.H{"Title": "Forbidden", "Username": username, "Role": roleVal})
			return
		}
		orgID, _ := c.Cookie("org")
		orgName, _ := c.Cookie("orgName")
		push := false
		if v, _ := c.Get("user:canPush"); v == true {
			push = true
		}
		c.HTML(http.StatusOK, "browse.html", gin.H{
			"Title":     "Browse Objects",
			"Username":  username,
			"Role":      roleVal,
			"OrgID":     orgID,
			"OrgName":   orgName,
			"StorageID": c.Param("storageId"),
			"CanPush":   push,
			"CanPull":   true,
			"HasRole":   func() bool { v, _ := c.Get("user:hasRole"); return v == true }(),
		})
	})

	// API Docs page (auth required), embeds Swagger UI inside the app layout
	app.GET("/docs", simpleAuthMiddleware(), func(c *gin.Context) {
		username, _ := c.Get("user:name")
		roleVal, _ := c.Get("user:role")
		if ok, _ := c.Get("user:hasRole"); ok != true {
			c.HTML(http.StatusForbidden, "forbidden.html", gin.H{"Title": "Forbidden", "Username": username, "Role": roleVal})
			return
		}
		orgID, _ := c.Cookie("org")
		orgName, _ := c.Cookie("orgName")
		c.HTML(http.StatusOK, "api_docs.html", gin.H{
			"Title":    "API Documentation",
			"Username": username,
			"Role":     roleVal,
			"OrgID":    orgID,
			"OrgName":  orgName,
			"CanPull":  func() bool { v, _ := c.Get("user:canPull"); return v == true }(),
			"CanPush":  func() bool { v, _ := c.Get("user:canPush"); return v == true }(),
			"HasRole":  true,
		})
	})

	// My Profile page (auth required)
	app.GET("/profile", simpleAuthMiddleware(), func(c *gin.Context) {
		username, _ := c.Get("user:name")
		roleVal, _ := c.Get("user:role")
		orgID, _ := c.Cookie("org")
		orgName, _ := c.Cookie("orgName")
		c.HTML(http.StatusOK, "profile.html", gin.H{
			"Title":    "My Profile",
			"Username": username,
			"Role":     roleVal,
			"OrgID":    orgID,
			"OrgName":  orgName,
			"CanPull":  func() bool { v, _ := c.Get("user:canPull"); return v == true }(),
			"CanPush":  func() bool { v, _ := c.Get("user:canPush"); return v == true }(),
			"HasRole":  func() bool { v, _ := c.Get("user:hasRole"); return v == true }(),
		})
	})
}
