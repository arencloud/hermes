package models

import (
	"time"
)

type User struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	Email               string    `gorm:"uniqueIndex" json:"email"`
	Password            string    `json:"-"`
	Role                string    `json:"role"`
	MustChangePassword  bool      `json:"mustChangePassword"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type Provider struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"` // aws|minio|mcg|generic
	Endpoint  string    `json:"endpoint"`
	AccessKey string    `json:"accessKey"`
	SecretKey string    `json:"secretKey"`
	Region    string    `json:"region"`
	UseSSL    bool      `json:"useSSL"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type AuthConfig struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	Mode             string    `json:"mode"` // local|oidc|saml
	Enabled          bool      `json:"enabled"`
	// OIDC config
	OIDCIssuer       string    `json:"oidcIssuer"`
	OIDCClientID     string    `json:"oidcClientId"`
	OIDCClientSecret string    `json:"oidcClientSecret"`
	OIDCScope        string    `json:"oidcScope"` // space-separated scopes
	OIDCRedirectURL  string    `json:"oidcRedirectUrl"`
	// OIDC claims mapping
	OIDCRoleClaim         string `json:"oidcRoleClaim"`   // e.g., "role" or custom namespace claim
	OIDCGroupClaim        string `json:"oidcGroupClaim"`  // e.g., "groups"
	OIDCAdminValues       string `json:"oidcAdminValues"` // comma/space separated values mapped to admin
	OIDCEditorValues      string `json:"oidcEditorValues"`
	OIDCViewerValues      string `json:"oidcViewerValues"`
	OIDCUpdateRoleOnLogin bool   `json:"oidcUpdateRoleOnLogin"`
	// SAML config (stored for future SAML flow)
	SAMLMetadataURL  string    `json:"samlMetadataUrl"`
	SAMLRoleClaim    string    `json:"samlRoleClaim"`
	SAMLGroupClaim   string    `json:"samlGroupClaim"`
	SAMLAdminValues  string    `json:"samlAdminValues"`
	SAMLEditorValues string    `json:"samlEditorValues"`
	SAMLViewerValues string    `json:"samlViewerValues"`
	// Defaults
	DefaultRole      string    `json:"defaultRole"` // role for new federated users
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// Persistent observability models

type LogEntry struct {
	ID     uint      `gorm:"primaryKey" json:"id"`
	Time   time.Time `json:"time"`
	Level  string    `json:"level"`
	Msg    string    `json:"msg"`
	Fields string    `json:"fields"` // JSON string of fields
}

type TraceRow struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	UserEmail string    `json:"userEmail"`
	UserRole  string    `json:"userRole"`
	UserAgent string    `json:"userAgent"`
	RemoteIP  string    `json:"remoteIp"`
	ReqBytes  int64     `json:"reqBytes"`
	RespBytes int64     `json:"respBytes"`
	Started   time.Time `json:"started"`
	Ended     time.Time `json:"ended"`
	DurationNs int64    `json:"durationNs"`
}

type TraceEventRow struct {
	ID      uint      `gorm:"primaryKey" json:"id"`
	TraceID string    `gorm:"index" json:"traceId"`
	Time    time.Time `json:"time"`
	Name    string    `json:"name"`
	Fields  string    `json:"fields"` // JSON string of fields
}
