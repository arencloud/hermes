package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// pg is a global DB handle used by the auth store when configured.
var pg *sql.DB

// InitPostgres configures the auth package to use PostgreSQL for user storage.
func InitPostgres(db *sql.DB) { pg = db }

// MigratePostgres creates necessary tables if they do not already exist.
func MigratePostgres(ctx context.Context) error {
	if pg == nil {
		return nil
	}
	_, err := pg.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			password_hash BYTEA NOT NULL,
			role TEXT NOT NULL,
			tenant TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	return err
}

// seedAdmin ensures an admin user exists with a given password.
func seedAdmin(ctx context.Context, username, password, tenant string) error {
	if pg == nil {
		return nil
	}
	// check if exists
	var exists bool
	err := pg.QueryRowContext(ctx, `SELECT true FROM users WHERE username=$1`, username).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if exists {
		return nil
	}
	return GetStore().CreateUser(username, password, "admin", tenant)
}

// --- PostgreSQL-backed implementations used by Store methods when pg != nil ---

func dbCreateUser(ctx context.Context, username, password string, role, tenant string) error {
	if username == "" || password == "" {
		return errors.New("username and password required")
	}
	if role == "" {
		role = "user"
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	_, err = pg.ExecContext(ctx, `INSERT INTO users (username, password_hash, role, tenant) VALUES ($1,$2,$3,$4)`, username, hash, role, tenant)
	if err != nil {
		// map unique to ErrUserExists
		return err
	}
	return nil
}

func dbUpdateUser(ctx context.Context, username, role, tenant, newPassword string) error {
	// Protect admin role
	if username == "admin" && role != "" && role != "admin" {
		return errors.New("cannot change admin role")
	}
	// Build dynamic update
	tx, err := pg.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// ensure exists
	var id int
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username=$1`, username).Scan(&id)
	if err != nil {
		return ErrUserNotFound
	}
	if newPassword != "" {
		hash, err := hashPassword(newPassword)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE users SET password_hash=$1 WHERE id=$2`, hash, id); err != nil {
			return err
		}
	}
	// role
	if role != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE users SET role=$1 WHERE id=$2`, role, id); err != nil {
			return err
		}
	}
	// tenant can be empty string to clear
	if _, err = tx.ExecContext(ctx, `UPDATE users SET tenant=$1 WHERE id=$2`, tenant, id); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func dbDeleteUser(ctx context.Context, username string) error {
	res, err := pg.ExecContext(ctx, `DELETE FROM users WHERE username=$1`, username)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrUserNotFound
	}
	return nil
}

func dbAuthenticate(ctx context.Context, username, password string) (*User, error) {
	var u User
	var hash []byte
	err := pg.QueryRowContext(ctx, `SELECT username, password_hash, role, tenant FROM users WHERE username=$1`, username).Scan(&u.Username, &hash, &u.Role, &u.Tenant)
	if err != nil {
		return nil, ErrInvalidCreds
	}
	if err := comparePassword(hash, []byte(password)); err != nil {
		return nil, ErrInvalidCreds
	}
	return &User{Username: u.Username, Role: u.Role, Tenant: u.Tenant}, nil
}

func dbGet(ctx context.Context, username string) (*User, bool) {
	var u User
	err := pg.QueryRowContext(ctx, `SELECT username, role, tenant FROM users WHERE username=$1`, username).Scan(&u.Username, &u.Role, &u.Tenant)
	if err != nil {
		return nil, false
	}
	return &u, true
}

func dbList(ctx context.Context) []*User {
	rows, err := pg.QueryContext(ctx, `SELECT username, role, tenant FROM users ORDER BY username`)
	if err != nil {
		return []*User{}
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.Username, &u.Role, &u.Tenant); err == nil {
			out = append(out, &u)
		}
	}
	_ = rows.Err()
	return out
}

// convenience helpers (local bcrypt usage)
func hashPassword(pw string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
}
func comparePassword(hash, pw []byte) error { return bcrypt.CompareHashAndPassword(hash, pw) }

// expose time for potential timeouts usage in callers
var defaultTimeout = 5 * time.Second
