package database

import (
	"fmt"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/arencloud/hermes/internal/models"
)

func dsnFromEnv() string {
	host := getenv("DB_HOST", "localhost")
	port := getenv("DB_PORT", "5432")
	user := getenv("DB_USER", "postgres")
	pass := getenv("DB_PASSWORD", "postgres")
	dbname := getenv("DB_NAME", "hermes")
	sslmode := getenv("DB_SSLMODE", "disable")
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, pass, dbname, sslmode)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func Connect() (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsnFromEnv()), &gorm.Config{})
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.Organization{},
		&models.User{},
		&models.Group{},
		&models.Role{},
		&models.UserGroup{},
		&models.UserRole{},
		&models.GroupRole{},
		&models.S3Storage{},
		&models.UserProfile{},
	)
}
