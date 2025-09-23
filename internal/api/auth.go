package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/arencloud/hermes/internal/db"
	"github.com/arencloud/hermes/internal/models"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
)

// very small in-memory session store
var sessions = make(map[string]uint) // sessionID -> userID
var secret = []byte("hermes-dev-secret")

func sign(value string) string {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func setSessionCookie(w http.ResponseWriter, sessionID string) {
	cookie := &http.Cookie{Name: "dsess", Value: sessionID + "." + sign(sessionID), Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(24 * time.Hour)}
	http.SetCookie(w, cookie)
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "dsess", Value: "", Path: "/", Expires: time.Unix(0, 0), MaxAge: -1})
}

func currentUser(r *http.Request) *models.User {
	c, err := r.Cookie("dsess")
	if err != nil {
		return nil
	}
	parts := c.Value
	var sid, sig string
	for i := 0; i < len(parts); i++ {
		if parts[i] == '.' {
			sid = parts[:i]
			sig = parts[i+1:]
			break
		}
	}
	if sid == "" || sig == "" || sign(sid) != sig {
		return nil
	}
	uid, ok := sessions[sid]
	if !ok {
		return nil
	}
	var u models.User
	if err := db.DB.First(&u, uid).Error; err != nil {
		return nil
	}
	return &u
}

func requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := currentUser(r); u != nil {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

func requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u == nil {
			http.Error(w, "unauthorized", 401)
			return
		}
		if u.Role != "admin" {
			http.Error(w, "forbidden", 403)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireEditorOrAdmin allows roles admin and editor; viewers are read-only
func requireEditorOrAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u == nil {
			http.Error(w, "unauthorized", 401)
			return
		}
		if u.Role != "admin" && u.Role != "editor" {
			http.Error(w, "forbidden", 403)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func registerAuth(r chi.Router, logger interface{}) {
	r.Route("/auth", func(r chi.Router) {
		r.Post("/login", login)
		r.Post("/change-password", changePassword)
		r.Get("/me", me)
		r.Post("/logout", logout)
		// Bootstrap status (unauthenticated): whether default admin must change password
		r.Get("/bootstrap", authBootstrap)
		// Public (unauthenticated) sanitized federation config for Login screen (no secrets)
		r.Get("/fed/public", getAuthConfigPublic)
		// Federation config (admin-only) and OIDC endpoints under /auth
		r.Group(func(ar chi.Router) {
			ar.Use(requireAuth)
			ar.Use(requireAdmin)
			ar.Get("/fed/config", getAuthConfig)
			ar.Put("/fed/config", updateAuthConfig)
		})
		r.Get("/oidc/start", oidcStart)
		r.Get("/oidc/callback", oidcCallback)
	})
}

func login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var in struct{ Email, Password string }
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var u models.User
	if err := db.DB.Where("email = ?", in.Email).First(&u).Error; err != nil {
		http.Error(w, "invalid credentials", 401)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(in.Password)) != nil {
		http.Error(w, "invalid credentials", 401)
		return
	}
	// create session
	sid := base64.RawURLEncoding.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano) + in.Email))
	sessions[sid] = u.ID
	setSessionCookie(w, sid)
	json.NewEncoder(w).Encode(map[string]any{"id": u.ID, "email": u.Email, "role": u.Role, "mustChangePassword": u.MustChangePassword})
}

func changePassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	u := currentUser(r)
	if u == nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct{ OldPassword, NewPassword string }
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if len(in.NewPassword) < 8 {
		http.Error(w, "password too short", 400)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(in.OldPassword)) != nil {
		http.Error(w, "invalid old password", 400)
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost)
	u.Password = string(hash)
	u.MustChangePassword = false
	if err := db.DB.Save(u).Error; err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	u := currentUser(r)
	if u == nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"id": u.ID, "email": u.Email, "role": u.Role, "mustChangePassword": u.MustChangePassword})
}

func logout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("dsess")
	if err == nil {
		val := c.Value
		var sid string
		for i := 0; i < len(val); i++ {
			if val[i] == '.' {
				sid = val[:i]
				break
			}
		}
		if sid != "" {
			delete(sessions, sid)
		}
	}
	clearSessionCookie(w)
	w.WriteHeader(204)
}

// authBootstrap returns whether the default bootstrap admin still must change password.
// This allows the UI to conditionally show the temporary password notice on first run.
func authBootstrap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var u models.User
	// Look up the default admin created at bootstrap
	show := db.DB.Where("email = ? AND must_change_password = ?", "admin@local", true).First(&u).Error == nil
	json.NewEncoder(w).Encode(map[string]any{"showTempNotice": show})
}

// -------- Federation config (admin) --------
func getAuthConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var ac models.AuthConfig
	if err := db.DB.First(&ac).Error; err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(ac)
}

// getAuthConfigPublic returns a sanitized subset of federation config for the Login page without secrets.
func getAuthConfigPublic(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var ac models.AuthConfig
	if err := db.DB.First(&ac).Error; err != nil {
		// Don't leak internal errors; return minimal default
		json.NewEncoder(w).Encode(map[string]any{"enabled": false, "mode": "local"})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"enabled":         ac.Enabled,
		"mode":            ac.Mode,
		"oidcIssuer":      ac.OIDCIssuer,
		"oidcClientId":    ac.OIDCClientID,
		"oidcScope":       ac.OIDCScope,
		"oidcRedirectUrl": ac.OIDCRedirectURL,
		"defaultRole":     ac.DefaultRole,
	})
}

func updateAuthConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var ac models.AuthConfig
	if err := db.DB.First(&ac).Error; err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var in map[string]any
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if v, ok := in["mode"].(string); ok {
		ac.Mode = v
	}
	if v, ok := in["enabled"].(bool); ok {
		ac.Enabled = v
	}
	if v, ok := in["oidcIssuer"].(string); ok {
		ac.OIDCIssuer = v
	}
	if v, ok := in["oidcClientId"].(string); ok {
		ac.OIDCClientID = v
	}
	if v, ok := in["oidcClientSecret"].(string); ok {
		ac.OIDCClientSecret = v
	}
	if v, ok := in["oidcScope"].(string); ok {
		ac.OIDCScope = v
	}
	if v, ok := in["oidcRedirectUrl"].(string); ok {
		ac.OIDCRedirectURL = v
	}
	if v, ok := in["samlMetadataUrl"].(string); ok {
		ac.SAMLMetadataURL = v
	}
	// OIDC/SAML mapping fields
	if v, ok := in["oidcRoleClaim"].(string); ok {
		ac.OIDCRoleClaim = v
	}
	if v, ok := in["oidcGroupClaim"].(string); ok {
		ac.OIDCGroupClaim = v
	}
	if v, ok := in["oidcAdminValues"].(string); ok {
		ac.OIDCAdminValues = v
	}
	if v, ok := in["oidcEditorValues"].(string); ok {
		ac.OIDCEditorValues = v
	}
	if v, ok := in["oidcViewerValues"].(string); ok {
		ac.OIDCViewerValues = v
	}
	if v, ok := in["oidcUpdateRoleOnLogin"].(bool); ok {
		ac.OIDCUpdateRoleOnLogin = v
	}
	if v, ok := in["samlRoleClaim"].(string); ok {
		ac.SAMLRoleClaim = v
	}
	if v, ok := in["samlGroupClaim"].(string); ok {
		ac.SAMLGroupClaim = v
	}
	if v, ok := in["samlAdminValues"].(string); ok {
		ac.SAMLAdminValues = v
	}
	if v, ok := in["samlEditorValues"].(string); ok {
		ac.SAMLEditorValues = v
	}
	if v, ok := in["samlViewerValues"].(string); ok {
		ac.SAMLViewerValues = v
	}
	if v, ok := in["defaultRole"].(string); ok {
		ac.DefaultRole = v
	}
	if ac.DefaultRole == "" {
		ac.DefaultRole = "viewer"
	}
	if err := db.DB.Save(&ac).Error; err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(ac)
}

// -------- OIDC flow --------
func oidcStart(w http.ResponseWriter, r *http.Request) {
	var ac models.AuthConfig
	if err := db.DB.First(&ac).Error; err != nil {
		http.Error(w, "not configured", 400)
		return
	}
	if !ac.Enabled || ac.Mode != "oidc" {
		http.Error(w, "oidc not enabled", 400)
		return
	}
	if ac.OIDCIssuer == "" || ac.OIDCClientID == "" || ac.OIDCRedirectURL == "" {
		http.Error(w, "missing oidc parameters", 400)
		return
	}
	conf := oauth2.Config{ClientID: ac.OIDCClientID, ClientSecret: ac.OIDCClientSecret, RedirectURL: ac.OIDCRedirectURL, Scopes: strings.Fields(ac.OIDCScope)}
	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, ac.OIDCIssuer)
	if err != nil {
		http.Error(w, "failed to discover issuer: "+err.Error(), 500)
		return
	}
	var ep oauth2.Endpoint
	if err := provider.Claims(&ep); err == nil {
		conf.Endpoint = ep
	}
	state := randToken(24)
	nonce := randToken(24)
	setTempCookie(w, "ds_oidc_state", state)
	setTempCookie(w, "ds_oidc_nonce", nonce)
	if len(conf.Scopes) == 0 {
		conf.Scopes = []string{"openid", "email", "profile"}
	}
	u := conf.AuthCodeURL(state, oidc.Nonce(nonce))
	http.Redirect(w, r, u, http.StatusFound)
}

func oidcCallback(w http.ResponseWriter, r *http.Request) {
	var ac models.AuthConfig
	if err := db.DB.First(&ac).Error; err != nil {
		http.Error(w, "not configured", 400)
		return
	}
	if !ac.Enabled || ac.Mode != "oidc" {
		http.Error(w, "oidc not enabled", 400)
		return
	}
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		http.Error(w, "invalid callback", 400)
		return
	}
	if !checkTempCookie(r, "ds_oidc_state", state) {
		http.Error(w, "state mismatch", 400)
		return
	}
	nonce := getTempCookie(r, "ds_oidc_nonce")
	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, ac.OIDCIssuer)
	if err != nil {
		http.Error(w, "issuer discovery failed", 500)
		return
	}
	conf := oauth2.Config{ClientID: ac.OIDCClientID, ClientSecret: ac.OIDCClientSecret, RedirectURL: ac.OIDCRedirectURL, Scopes: strings.Fields(ac.OIDCScope)}
	var ep oauth2.Endpoint
	if err := provider.Claims(&ep); err == nil {
		conf.Endpoint = ep
	}
	tok, err := conf.Exchange(ctx, code)
	if err != nil {
		http.Error(w, "token exchange failed", 400)
		return
	}
	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		http.Error(w, "missing id_token", 400)
		return
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: ac.OIDCClientID})
	idTok, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		http.Error(w, "invalid id_token", 400)
		return
	}
	var claims struct {
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
		Nonce             string `json:"nonce"`
	}
	_ = idTok.Claims(&claims)
	if nonce != "" && claims.Nonce != "" && claims.Nonce != nonce {
		http.Error(w, "nonce mismatch", 400)
		return
	}
	// Extract full claim set for role/group mapping
	var raw map[string]any
	_ = idTok.Claims(&raw)
	email := strings.ToLower(strings.TrimSpace(firstNonEmpty(claims.Email, claims.PreferredUsername)))
	if email == "" {
		http.Error(w, "email claim required", 400)
		return
	}
	// Derive role from configured claim mapping
	mappedRole := mapClaimsToRole(raw, ac)
	if mappedRole == "" {
		mappedRole = defaultRole(ac.DefaultRole)
	}
	var u models.User
	if err := db.DB.Where("email = ?", email).First(&u).Error; err != nil {
		u = models.User{Email: email, Role: mappedRole}
		if err := db.DB.Create(&u).Error; err != nil {
			http.Error(w, "failed to create user", 500)
			return
		}
	} else if ac.OIDCUpdateRoleOnLogin {
		// Update existing user's role on login if enabled
		u.Role = mappedRole
		_ = db.DB.Save(&u).Error
	}
	sid := base64.RawURLEncoding.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano) + email))
	sessions[sid] = u.ID
	setSessionCookie(w, sid)
	http.Redirect(w, r, "/#/dashboard", http.StatusFound)
}

func defaultRole(r string) string {
	s := strings.TrimSpace(strings.ToLower(r))
	if s == "admin" || s == "editor" || s == "viewer" {
		return s
	}
	return "viewer"
}

func setTempCookie(w http.ResponseWriter, name, val string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: url.QueryEscape(val), Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(10 * time.Minute)})
}
func getTempCookie(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	v, _ := url.QueryUnescape(c.Value)
	return v
}
func checkTempCookie(r *http.Request, name, expected string) bool {
	return getTempCookie(r, name) == expected
}

func randToken(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// mapClaimsToRole determines app role based on configured OIDC/SAML mapping in AuthConfig.
// It checks OIDC first (for current flow). Values can be a string, []string or []any.
func mapClaimsToRole(claims map[string]any, ac models.AuthConfig) string {
	vals := func(v any) []string {
		s := []string{}
		switch t := v.(type) {
		case string:
			s = append(s, t)
		case []any:
			for _, x := range t {
				if xs, ok := x.(string); ok {
					s = append(s, xs)
				}
			}
		case []string:
			s = append(s, t...)
		}
		for i := range s {
			s[i] = strings.ToLower(strings.TrimSpace(s[i]))
		}
		return s
	}
	containsAny := func(set []string, needles []string) bool {
		m := map[string]struct{}{}
		for _, v := range set {
			m[v] = struct{}{}
		}
		for _, n := range needles {
			if _, ok := m[strings.ToLower(strings.TrimSpace(n))]; ok {
				return true
			}
		}
		return false
	}
	// Collect claimed role/group values
	var roleVals, groupVals []string
	if ac.OIDCRoleClaim != "" {
		if v, ok := claims[ac.OIDCRoleClaim]; ok {
			roleVals = vals(v)
		}
	}
	if ac.OIDCGroupClaim != "" {
		if v, ok := claims[ac.OIDCGroupClaim]; ok {
			groupVals = vals(v)
		}
	}
	combined := append([]string{}, roleVals...)
	combined = append(combined, groupVals...)
	// Parse config lists
	split := func(s string) []string {
		if s == "" {
			return nil
		}
		parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\r' || r == '\t' })
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.ToLower(strings.TrimSpace(p))
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	admins := split(ac.OIDCAdminValues)
	editors := split(ac.OIDCEditorValues)
	viewers := split(ac.OIDCViewerValues)
	// Priority: admin > editor > viewer
	if containsAny(combined, admins) {
		return "admin"
	}
	if containsAny(combined, editors) {
		return "editor"
	}
	if containsAny(combined, viewers) {
		return "viewer"
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
