package controllers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/arencloud/hermes/internal/models"
	"github.com/arencloud/hermes/internal/s3svc"
)

// Helpers
func orgIDParam(c *gin.Context) (uint, error) {
	// Prefer organization from logged-in context (cookie), fallback to path parameter
	if orgCookie, err := c.Cookie("org"); err == nil && orgCookie != "" {
		if id, perr := strconv.ParseUint(orgCookie, 10, 64); perr == nil && id > 0 {
			return uint(id), nil
		}
	}
	idStr := c.Param("orgId")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("invalid orgId")
	}
	return uint(id), nil
}

func idParam(c *gin.Context) (uint, error) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("invalid id")
	}
	return uint(id), nil
}

// Organizations

func ListOrganizations(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var list []models.Organization
		if err := db.Find(&list).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, list)
	}
}

func CreateOrganization(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body models.Organization
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// Generate slug if missing and ensure uniqueness
		if body.Slug == "" {
			base := slugify(body.Name)
			slug, err := ensureUniqueOrgSlug(db, base)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			body.Slug = slug
		}
		if err := db.Create(&body).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Ensure default Super Admin role exists for this organization
		_ = db.Where("organization_id = ? AND name = ?", body.ID, "Super Admin").FirstOrCreate(&models.Role{
			OrganizationID: body.ID,
			Name:           "Super Admin",
			Key:            "super_admin",
			IsSystem:       true,
			Description:    "Has full access within the organization",
		}).Error
		c.JSON(http.StatusCreated, body)
	}
}

// slugify converts a string into a URL-friendly slug
func slugify(s string) string {
	// lightweight slug: lowercase, alnum to itself, others to '-'; collapse dashes
	out := make([]rune, 0, len(s))
	prevDash := false
	for _, r := range []rune(s) {
		if r >= 'A' && r <= 'Z' {
			r = r + 32
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out = append(out, r)
			prevDash = false
			continue
		}
		if !prevDash {
			out = append(out, '-')
			prevDash = true
		}
	}
	// trim leading/trailing '-'
	start, end := 0, len(out)
	for start < end && out[start] == '-' {
		start++
	}
	for end > start && out[end-1] == '-' {
		end--
	}
	if start >= end {
		return "org"
	}
	return string(out[start:end])
}

// ensureUniqueOrgSlug ensures the slug is unique by appending -2, -3, ... if needed
func ensureUniqueOrgSlug(db *gorm.DB, base string) (string, error) {
	slug := base
	if slug == "" {
		slug = "org"
	}
	var exists models.Organization
	idx := 1
	for {
		if err := db.Where("slug = ?", slug).First(&exists).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return slug, nil
			}
			return "", err
		}
		idx++
		slug = fmt.Sprintf("%s-%d", base, idx)
	}
}

func GetOrganization(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var org models.Organization
		if err := db.First(&org, orgID).Error; err != nil {
			status := http.StatusNotFound
			if err != gorm.ErrRecordNotFound {
				status = http.StatusInternalServerError
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, org)
	}
}

func UpdateOrganization(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var body models.Organization
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		body.ID = orgID
		if err := db.Model(&models.Organization{ID: orgID}).Updates(body).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, body)
	}
}

func DeleteOrganization(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := db.Delete(&models.Organization{}, orgID).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// Users
func ListUsers(db *gorm.DB) gin.HandlerFunc { return listScoped[models.User](db) }

// User Profile
func GetUserProfile(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		userID, err := idParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var up models.UserProfile
		if err := db.Where("organization_id = ? AND user_id = ?", orgID, userID).First(&up).Error; err != nil {
			status := http.StatusNotFound
			if err != gorm.ErrRecordNotFound {
				status = http.StatusInternalServerError
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, up)
	}
}

func UpdateUserProfile(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		userID, err := idParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var body struct {
			FirstName *string `json:"firstName"`
			LastName  *string `json:"lastName"`
			Phone     *string `json:"phone"`
			AvatarURL *string `json:"avatarUrl"`
			Bio       *string `json:"bio"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// Ensure a profile exists
		var up models.UserProfile
		if err := db.Where("organization_id = ? AND user_id = ?", orgID, userID).First(&up).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				up = models.UserProfile{OrganizationID: orgID, UserID: userID}
				if err := db.Create(&up).Error; err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		updates := map[string]any{"organization_id": orgID, "user_id": userID}
		if body.FirstName != nil {
			updates["first_name"] = *body.FirstName
		}
		if body.LastName != nil {
			updates["last_name"] = *body.LastName
		}
		if body.Phone != nil {
			updates["phone"] = *body.Phone
		}
		if body.AvatarURL != nil {
			updates["avatar_url"] = *body.AvatarURL
		}
		if body.Bio != nil {
			updates["bio"] = *body.Bio
		}
		if err := db.Model(&models.UserProfile{}).
			Where("organization_id = ? AND user_id = ?", orgID, userID).
			Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if err := db.Where("organization_id = ? AND user_id = ?", orgID, userID).First(&up).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, up)
	}
}

// CreateUser hashes password and stores PasswordHash
func CreateUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var body struct {
			Email       string `json:"email"`
			DisplayName string `json:"displayName"`
			Password    string `json:"password"`
			IsActive    *bool  `json:"isActive"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if body.Email == "" || body.Password == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "email and password are required"})
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		u := models.User{
			OrganizationID: orgID,
			Email:          body.Email,
			DisplayName:    body.DisplayName,
			PasswordHash:   string(hash),
		}
		if body.IsActive != nil {
			u.IsActive = *body.IsActive
		}
		if err := db.Create(&u).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, u)
	}
}

func GetUser(db *gorm.DB) gin.HandlerFunc { return getScoped[models.User](db) }

// UpdateUser allows changing email, displayName, isActive, and password (optional)
func UpdateUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		id, err := idParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var body struct {
			Email       *string `json:"email"`
			DisplayName *string `json:"displayName"`
			Password    *string `json:"password"`
			IsActive    *bool   `json:"isActive"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		updates := map[string]any{"organization_id": orgID}
		if body.Email != nil {
			updates["email"] = *body.Email
		}
		if body.DisplayName != nil {
			updates["display_name"] = *body.DisplayName
		}
		if body.IsActive != nil {
			updates["is_active"] = *body.IsActive
		}
		if body.Password != nil && *body.Password != "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(*body.Password), bcrypt.DefaultCost)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			updates["password_hash"] = string(hash)
		}
		var u models.User
		if err := db.Model(&u).Where("id = ? AND organization_id = ?", id, orgID).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if err := db.Where("organization_id = ?", orgID).First(&u, id).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, u)
	}
}

func DeleteUser(db *gorm.DB) gin.HandlerFunc { return deleteScoped[models.User](db) }

// Groups
func ListGroups(db *gorm.DB) gin.HandlerFunc  { return listScoped[models.Group](db) }
func CreateGroup(db *gorm.DB) gin.HandlerFunc { return createScoped[models.Group](db) }
func GetGroup(db *gorm.DB) gin.HandlerFunc    { return getScoped[models.Group](db) }
func UpdateGroup(db *gorm.DB) gin.HandlerFunc { return updateScoped[models.Group](db) }
func DeleteGroup(db *gorm.DB) gin.HandlerFunc { return deleteScoped[models.Group](db) }

// Roles
func ListRoles(db *gorm.DB) gin.HandlerFunc  { return listScoped[models.Role](db) }
func CreateRole(db *gorm.DB) gin.HandlerFunc { return createScoped[models.Role](db) }
func GetRole(db *gorm.DB) gin.HandlerFunc    { return getScoped[models.Role](db) }
func UpdateRole(db *gorm.DB) gin.HandlerFunc { return updateScoped[models.Role](db) }
func DeleteRole(db *gorm.DB) gin.HandlerFunc { return deleteScoped[models.Role](db) }

// Memberships (UserGroup)
func ListMemberships(db *gorm.DB) gin.HandlerFunc  { return listScoped[models.UserGroup](db) }
func CreateMembership(db *gorm.DB) gin.HandlerFunc { return createScoped[models.UserGroup](db) }
func GetMembership(db *gorm.DB) gin.HandlerFunc    { return getScoped[models.UserGroup](db) }
func DeleteMembership(db *gorm.DB) gin.HandlerFunc { return deleteScoped[models.UserGroup](db) }

// Role Assignments (UserRole)
func ListRoleAssignments(db *gorm.DB) gin.HandlerFunc  { return listScoped[models.UserRole](db) }
func CreateRoleAssignment(db *gorm.DB) gin.HandlerFunc { return createScoped[models.UserRole](db) }
func GetRoleAssignment(db *gorm.DB) gin.HandlerFunc    { return getScoped[models.UserRole](db) }
func DeleteRoleAssignment(db *gorm.DB) gin.HandlerFunc { return deleteScoped[models.UserRole](db) }

// Group Role Assignments (GroupRole)
func ListGroupRoleAssignments(db *gorm.DB) gin.HandlerFunc { return listScoped[models.GroupRole](db) }
func CreateGroupRoleAssignment(db *gorm.DB) gin.HandlerFunc {
	return createScoped[models.GroupRole](db)
}
func GetGroupRoleAssignment(db *gorm.DB) gin.HandlerFunc { return getScoped[models.GroupRole](db) }
func DeleteGroupRoleAssignment(db *gorm.DB) gin.HandlerFunc {
	return deleteScoped[models.GroupRole](db)
}

// Storages (config CRUD with ownership enforcement)
func ListStorages(db *gorm.DB) gin.HandlerFunc  { return listScoped[models.S3Storage](db) }

func CreateStorage(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var body models.S3Storage
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		body.OrganizationID = orgID
		// set ownership from current cookie user within org
		if email, err := c.Cookie("user"); err == nil && email != "" {
			var u models.User
			if db.Where("organization_id = ? AND email = ?", orgID, email).First(&u).Error == nil {
				body.CreatedByUserID = u.ID
				body.CreatedByEmail = u.Email
			}
		}
		if err := db.Create(&body).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, body)
	}
}

func GetStorage(db *gorm.DB) gin.HandlerFunc    { return getScoped[models.S3Storage](db) }

func UpdateStorage(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil { c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}); return }
		id, err := idParam(c)
		if err != nil { c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}); return }
		// load existing to check ownership
		var existing models.S3Storage
		if err := db.Where("organization_id = ?", orgID).First(&existing, id).Error; err != nil {
			status := http.StatusNotFound
			if err != gorm.ErrRecordNotFound { status = http.StatusInternalServerError }
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		isAdmin := false
		if role, _ := c.Cookie("role"); role == "admin" { isAdmin = true }
		if !isAdmin {
			// determine current user
			email, _ := c.Cookie("user")
			var u models.User
			if err := db.Where("organization_id = ? AND email = ?", orgID, email).First(&u).Error; err != nil || u.ID != existing.CreatedByUserID {
				c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
				return
			}
		}
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil { c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}); return }
		// enforce scope and prevent changing ownership
		body["organization_id"] = orgID
		delete(body, "created_by_user_id")
		delete(body, "created_by_email")
		if err := db.Model(&models.S3Storage{}).Where("id = ? AND organization_id = ?", id, orgID).Updates(body).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		var updated models.S3Storage
		if err := db.Where("organization_id = ?", orgID).First(&updated, id).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, updated)
	}
}

func DeleteStorage(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil { c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}); return }
		id, err := idParam(c)
		if err != nil { c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}); return }
		var existing models.S3Storage
		if err := db.Where("organization_id = ?", orgID).First(&existing, id).Error; err != nil {
			status := http.StatusNotFound
			if err != gorm.ErrRecordNotFound { status = http.StatusInternalServerError }
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		isAdmin := false
		if role, _ := c.Cookie("role"); role == "admin" { isAdmin = true }
		if !isAdmin {
			email, _ := c.Cookie("user")
			var u models.User
			if err := db.Where("organization_id = ? AND email = ?", orgID, email).First(&u).Error; err != nil || u.ID != existing.CreatedByUserID {
				c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
				return
			}
		}
		if err := db.Delete(&existing).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// Generic scoped handlers using Go generics

type orgScoped interface{ ~struct{ OrganizationID uint } }

type ider interface{ ~struct{ ID uint } }

func listScoped[T any](db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var list []T
		if err := db.Where("organization_id = ?", orgID).Find(&list).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, list)
	}
}

func createScoped[T any](db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var body T
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// set OrganizationID if present
		switch v := any(&body).(type) {
		case *models.User:
			v.OrganizationID = orgID
		case *models.Group:
			v.OrganizationID = orgID
		case *models.Role:
			v.OrganizationID = orgID
		case *models.S3Storage:
			v.OrganizationID = orgID
		case *models.UserGroup:
			v.OrganizationID = orgID
		case *models.UserRole:
			v.OrganizationID = orgID
		case *models.GroupRole:
			v.OrganizationID = orgID
		}
		if err := db.Create(&body).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, body)
	}
}

func getScoped[T any](db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		id, err := idParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var obj T
		if err := db.Where("organization_id = ?", orgID).First(&obj, id).Error; err != nil {
			status := http.StatusNotFound
			if err != gorm.ErrRecordNotFound {
				status = http.StatusInternalServerError
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, obj)
	}
}

func updateScoped[T any](db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		id, err := idParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// ensure org scope cannot be changed
		body["organization_id"] = orgID
		var t T
		if err := db.Model(&t).Where("id = ? AND organization_id = ?", id, orgID).Updates(body).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// return updated
		if err := db.Where("organization_id = ?", orgID).First(&t, id).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, t)
	}
}

func deleteScoped[T any](db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		id, err := idParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var t T
		if err := db.Where("organization_id = ?", orgID).Delete(&t, id).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// S3 Object operations

func ListObjects(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		storageID, err := idParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		storage, err := loadStorage(db, orgID, storageID)
		if err != nil {
			respondStorageErr(c, err)
			return
		}
		client := s3svc.FromConfig(storage)
		ctx := context.Background()
		prefix := c.Query("prefix")
		objects, err := client.ListObjects(ctx, storage.Bucket, prefix)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, objects)
	}
}

func UploadObject(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		storageID, err := idParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		fileHeader, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
			return
		}
		key := c.DefaultPostForm("key", fileHeader.Filename)
		storage, err := loadStorage(db, orgID, storageID)
		if err != nil {
			respondStorageErr(c, err)
			return
		}
		src, err := fileHeader.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		defer src.Close()
		client := s3svc.FromConfig(storage)
		ctx := context.Background()
		if err := client.Upload(ctx, storage.Bucket, key, src, fileHeader.Size, "application/octet-stream"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"key": key})
	}
}

func DownloadObject(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		storageID, err := idParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		key := c.Param("key")
		if len(key) > 0 && key[0] == '/' {
			key = key[1:]
		}
		if key == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
			return
		}
		storage, err := loadStorage(db, orgID, storageID)
		if err != nil {
			respondStorageErr(c, err)
			return
		}
		client := s3svc.FromConfig(storage)
		ctx := context.Background()
		reader, length, contentType, err := client.Download(ctx, storage.Bucket, key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer reader.Close()
		c.Header("Content-Length", fmt.Sprintf("%d", length))
		if contentType != "" {
			c.Header("Content-Type", contentType)
		} else {
			c.Header("Content-Type", "application/octet-stream")
		}
		c.Header("Content-Disposition", "attachment; filename=\""+key+"\"")
		if _, err := io.Copy(c.Writer, reader); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
	}
}

func loadStorage(db *gorm.DB, orgID, storageID uint) (*models.S3Storage, error) {
	var st models.S3Storage
	if err := db.Where("organization_id = ?", orgID).First(&st, storageID).Error; err != nil {
		return nil, err
	}
	return &st, nil
}

func respondStorageErr(c *gin.Context, err error) {
	status := http.StatusNotFound
	if err != gorm.ErrRecordNotFound {
		status = http.StatusInternalServerError
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

// DeleteObject deletes an object from S3 storage
func DeleteObject(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := orgIDParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		storageID, err := idParam(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		key := c.Param("key")
		if len(key) > 0 && key[0] == '/' {
			key = key[1:]
		}
		if key == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
			return
		}
		storage, err := loadStorage(db, orgID, storageID)
		if err != nil {
			respondStorageErr(c, err)
			return
		}
		client := s3svc.FromConfig(storage)
		ctx := context.Background()
		if err := client.Delete(ctx, storage.Bucket, key); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}
