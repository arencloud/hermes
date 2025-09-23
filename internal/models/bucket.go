package models

import "time"

// Bucket represents a persisted view of buckets accessible by a Provider.
// Unique per (ProviderID, Name)
// We only store minimal info to keep schema simple and robust across vendors.
type Bucket struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ProviderID uint      `gorm:"index;not null" json:"providerId"`
	Name       string    `gorm:"not null" json:"name"`
	Region     string    `json:"region"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}
