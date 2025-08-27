package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/arencloud/hermes/internal/controllers"
	"github.com/arencloud/hermes/internal/version"
)

func RegisterAPIRoutes(app *gin.Engine, db *gorm.DB) {
	v1 := app.Group("/api/v1")

	// Simple cookie-based auth and permission middleware
	requireAuth := func(c *gin.Context) {
		if _, err := c.Cookie("auth"); err != nil { c.JSON(http.StatusUnauthorized, gin.H{"error":"unauthorized"}); c.Abort(); return }
		c.Next()
	}
	requirePull := func(c *gin.Context) {
		if v, err := c.Cookie("canPull"); err != nil || v != "1" { c.JSON(http.StatusForbidden, gin.H{"error":"forbidden"}); c.Abort(); return }
		c.Next()
	}
	requirePush := func(c *gin.Context) {
		if v, err := c.Cookie("canPush"); err != nil || v != "1" { c.JSON(http.StatusForbidden, gin.H{"error":"forbidden"}); c.Abort(); return }
		c.Next()
	}

	// organization-scoped resources
	org := v1.Group("/organizations/:orgId")
	org.Use(requireAuth)
	{
		org.GET("", controllers.GetOrganization(db))
		org.PUT("", controllers.UpdateOrganization(db))
		org.DELETE("", controllers.DeleteOrganization(db))

		orgUsers := org.Group("/users")
		{
			orgUsers.GET("", controllers.ListUsers(db))
			orgUsers.POST("", controllers.CreateUser(db))
	   orgUsers.GET("/:id", controllers.GetUser(db))
			orgUsers.PUT("/:id", controllers.UpdateUser(db))
			orgUsers.DELETE("/:id", controllers.DeleteUser(db))
			// User profile endpoints
			orgUsers.GET("/:id/profile", controllers.GetUserProfile(db))
			orgUsers.PUT("/:id/profile", controllers.UpdateUserProfile(db))
		}

		orgGroups := org.Group("/groups")
		{
			orgGroups.GET("", controllers.ListGroups(db))
			orgGroups.POST("", controllers.CreateGroup(db))
	   orgGroups.GET("/:id", controllers.GetGroup(db))
			orgGroups.PUT("/:id", controllers.UpdateGroup(db))
			orgGroups.DELETE("/:id", controllers.DeleteGroup(db))
		}

		orgRoles := org.Group("/roles")
		{
			orgRoles.GET("", controllers.ListRoles(db))
			orgRoles.POST("", controllers.CreateRole(db))
	   orgRoles.GET("/:id", controllers.GetRole(db))
			orgRoles.PUT("/:id", controllers.UpdateRole(db))
			orgRoles.DELETE("/:id", controllers.DeleteRole(db))
		}

		// Group memberships
		memberships := org.Group("/memberships")
		{
			memberships.GET("", controllers.ListMemberships(db))
			memberships.POST("", controllers.CreateMembership(db))
	   memberships.GET("/:id", controllers.GetMembership(db))
			memberships.DELETE("/:id", controllers.DeleteMembership(db))
		}

		// Role assignments
		assign := org.Group("/role-assignments")
		{
			assign.GET("", controllers.ListRoleAssignments(db))
			assign.POST("", controllers.CreateRoleAssignment(db))
		   assign.GET("/:id", controllers.GetRoleAssignment(db))
			assign.DELETE("/:id", controllers.DeleteRoleAssignment(db))
		}

		// Group Role assignments (assign roles to groups)
		groupAssign := org.Group("/group-role-assignments")
		{
			groupAssign.GET("", controllers.ListGroupRoleAssignments(db))
			groupAssign.POST("", controllers.CreateGroupRoleAssignment(db))
			groupAssign.GET("/:id", controllers.GetGroupRoleAssignment(db))
			groupAssign.DELETE("/:id", controllers.DeleteGroupRoleAssignment(db))
		}

		storages := org.Group("/storages")
		{
			// Listing storages requires pull; modifications require push
			storages.GET("", requirePull, controllers.ListStorages(db))
			storages.POST("", requirePush, controllers.CreateStorage(db))
	   storages.GET("/:id", requirePull, controllers.GetStorage(db))
			storages.PUT("/:id", requirePush, controllers.UpdateStorage(db))
			storages.DELETE("/:id", requirePush, controllers.DeleteStorage(db))

		   objects := storages.Group("/:id/objects")
			{
				objects.GET("", requirePull, controllers.ListObjects(db))
				objects.POST("", requirePush, controllers.UploadObject(db))
				objects.GET("/*key", requirePull, controllers.DownloadObject(db))
				objects.DELETE("/*key", requirePush, controllers.DeleteObject(db))
			}
		}
	}

	// top-level organizations CRUD
	v1.GET("/organizations", controllers.ListOrganizations(db))
	v1.POST("/organizations", controllers.CreateOrganization(db))

	// default route
	app.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	})
}

// Docs: serve openapi and swagger ui
func RegisterDocsRoutes(app *gin.Engine) {
	// Protect docs with auth and hasRole
	docsGuard := func(c *gin.Context) {
		if _, err := c.Cookie("auth"); err != nil { c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error":"unauthorized"}); return }
		if v, err := c.Cookie("hasRole"); err != nil || v != "1" { c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error":"forbidden"}); return }
		c.Next()
	}
	app.GET("/openapi.json", docsGuard, func(c *gin.Context) {
		c.File("openapi/openapi.json")
	})
	app.GET("/swagger", docsGuard, func(c *gin.Context) {
		// include app name/version in title
		title := version.AppName + " API v" + version.AppVersion + " · Swagger"
		html := "<!doctype html><html><head><title>" + title + "</title>\n" +
			"<link rel=\"stylesheet\" href=\"https://unpkg.com/swagger-ui-dist@5/swagger-ui.css\">\n" +
			"</head><body>\n" +
			"<div id=\"swagger-ui\"></div>\n" +
			"<script src=\"https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js\"></script>\n" +
			"<script>\n" +
			"window.ui = SwaggerUIBundle({ url: '/openapi.json', dom_id: '#swagger-ui' });\n" +
			"</script>\n" +
			"</body></html>"
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
	})
}
