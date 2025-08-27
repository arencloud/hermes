package models

import (
	"time"
)

type Organization struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"uniqueIndex;size:200;not null" json:"name"`
	Slug      string    `gorm:"uniqueIndex;size:200" json:"slug"`
	Domain    string    `gorm:"size:200" json:"domain"`
	Description string  `gorm:"size:500" json:"description"`
	IsActive  bool      `gorm:"default:true" json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type User struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	OrganizationID uint       `gorm:"index;not null" json:"organizationId"`
	Email          string     `gorm:"index;size:200;not null" json:"email"`
	DisplayName    string     `gorm:"size:200" json:"displayName"`
	PasswordHash   string     `gorm:"size:200;not null" json:"-"`
	IsActive       bool       `gorm:"default:true" json:"isActive"`
	LastLoginAt    *time.Time `json:"lastLoginAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type Group struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	OrganizationID uint      `gorm:"index;not null" json:"organizationId"`
	Name           string    `gorm:"size:200;not null" json:"name"`
	Description    string    `gorm:"size:500" json:"description"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Role struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	OrganizationID uint      `gorm:"index;not null" json:"organizationId"`
	Name           string    `gorm:"size:200;not null" json:"name"`
	Key            string    `gorm:"size:100" json:"key"`
	IsSystem       bool      `gorm:"default:false" json:"isSystem"`
	Description    string    `gorm:"size:500" json:"description"`
	CanPull        bool      `gorm:"default:false" json:"canPull"`
	CanPush        bool      `gorm:"default:false" json:"canPush"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// Join tables

type UserGroup struct {
	ID             uint `gorm:"primaryKey" json:"id"`
	OrganizationID uint `gorm:"index;not null" json:"organizationId"`
	UserID         uint `gorm:"index;not null" json:"userId"`
	GroupID        uint `gorm:"index;not null" json:"groupId"`
}

type UserRole struct {
	ID             uint `gorm:"primaryKey" json:"id"`
	OrganizationID uint `gorm:"index;not null" json:"organizationId"`
	UserID         uint `gorm:"index;not null" json:"userId"`
	RoleID         uint `gorm:"index;not null" json:"roleId"`
}

type GroupRole struct {
	ID             uint `gorm:"primaryKey" json:"id"`
	OrganizationID uint `gorm:"index;not null" json:"organizationId"`
	GroupID        uint `gorm:"index;not null" json:"groupId"`
	RoleID         uint `gorm:"index;not null" json:"roleId"`
}

// S3 storage configuration per organization

type S3Storage struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	OrganizationID uint      `gorm:"index;not null" json:"organizationId"`
	Name           string    `gorm:"size:200;not null" json:"name"`
	Endpoint       string    `gorm:"size:300;not null" json:"endpoint"`
	Region         string    `gorm:"size:100" json:"region"`
	AccessKey      string    `gorm:"size:200;not null" json:"accessKey"`
	SecretKey      string    `gorm:"size:500;not null" json:"secretKey"`
	UseSSL         bool      `json:"useSSL"`
	Bucket         string    `gorm:"size:200;not null" json:"bucket"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// UserProfile holds additional per-user optional information within an organization
// One profile per (organization_id, user_id)
// Note: kept separate from User to avoid impacting auth fields

type UserProfile struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	OrganizationID uint      `gorm:"index;uniqueIndex:uniq_user_profile;not null" json:"organizationId"`
	UserID         uint      `gorm:"index;uniqueIndex:uniq_user_profile;not null" json:"userId"`
	FirstName      string    `gorm:"size:100" json:"firstName"`
	LastName       string    `gorm:"size:100" json:"lastName"`
	Phone          string    `gorm:"size:50" json:"phone"`
	AvatarURL      string    `gorm:"size:500" json:"avatarUrl"`
	Bio            string    `gorm:"size:1000" json:"bio"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}
