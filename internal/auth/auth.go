package auth

import (
	"context"
	"errors"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	Username     string
	PasswordHash []byte
	Role         string // "admin", "developer" or "user"
	Tenant       string // optional tenant assignment
}

type Store struct {
	mu    sync.RWMutex
	users map[string]*User
}

func NewStore() *Store {
	return &Store{users: make(map[string]*User)}
}

var (
	ErrUserExists    = errors.New("user already exists")
	ErrUserNotFound  = errors.New("user not found")
	ErrInvalidCreds  = errors.New("invalid credentials")
)

func (s *Store) CreateUser(username, password, role, tenant string) error {
	if pg != nil {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
		return dbCreateUser(ctx, username, password, role, tenant)
	}
	if username == "" || password == "" {
		return errors.New("username and password required")
	}
	if role == "" {
		role = "user"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[username]; ok {
		return ErrUserExists
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	s.users[username] = &User{Username: username, PasswordHash: hash, Role: role, Tenant: tenant}
	return nil
}

func (s *Store) UpdateUser(username, role, tenant, newPassword string) error {
	if pg != nil {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
		return dbUpdateUser(ctx, username, role, tenant, newPassword)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[username]
	if !ok {
		return ErrUserNotFound
	}
	// Guard: don't demote built-in admin
	if username == "admin" && role != "admin" {
		return errors.New("cannot change admin role")
	}
	if role != "" {
		u.Role = role
	}
	// tenant can be empty to clear
	u.Tenant = tenant
	if newPassword != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		u.PasswordHash = hash
	}
	return nil
}

func (s *Store) DeleteUser(username string) error {
	if pg != nil {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
		return dbDeleteUser(ctx, username)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[username]; !ok {
		return ErrUserNotFound
	}
	delete(s.users, username)
	return nil
}

func (s *Store) Authenticate(username, password string) (*User, error) {
	if pg != nil {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
		return dbAuthenticate(ctx, username, password)
	}
	s.mu.RLock()
	u, ok := s.users[username]
	s.mu.RUnlock()
	if !ok {
		return nil, ErrInvalidCreds
	}
	if err := bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(password)); err != nil {
		return nil, ErrInvalidCreds
	}
	return &User{Username: u.Username, Role: u.Role, Tenant: u.Tenant}, nil
}

func (s *Store) Get(username string) (*User, bool) {
	if pg != nil {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
		return dbGet(ctx, username)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[username]
	if !ok {
		return nil, false
	}
	// return without hash for safety
	return &User{Username: u.Username, Role: u.Role, Tenant: u.Tenant}, true
}

func (s *Store) List() []*User {
	if pg != nil {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
		return dbList(ctx)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, &User{Username: u.Username, Role: u.Role, Tenant: u.Tenant})
	}
	return out
}
