package api

import (
	"github.com/arencloud/hermes/internal/models"
	"testing"
)

func TestFirstNonEmpty(t *testing.T) {
	if firstNonEmpty("", " ", "x") != "x" {
		t.Fatal("expected x")
	}
	if firstNonEmpty("a", "b") != "a" {
		t.Fatal("expected a")
	}
	if firstNonEmpty() != "" {
		t.Fatal("expected empty")
	}
}

func TestMapClaimsToRole(t *testing.T) {
	ac := models.AuthConfig{
		OIDCRoleClaim:    "role",
		OIDCGroupClaim:   "groups",
		OIDCAdminValues:  "admin, s3-admin",
		OIDCEditorValues: "editor",
		OIDCViewerValues: "viewer, read-only",
	}
	claims := map[string]any{"role": "editor", "groups": []any{"team", "dev"}}
	if r := mapClaimsToRole(claims, ac); r != "editor" {
		t.Fatalf("expected editor, got %s", r)
	}
	claims = map[string]any{"groups": []string{"read-only"}}
	if r := mapClaimsToRole(claims, ac); r != "viewer" {
		t.Fatalf("expected viewer, got %s", r)
	}
	claims = map[string]any{"role": "admin"}
	if r := mapClaimsToRole(claims, ac); r != "admin" {
		t.Fatalf("expected admin, got %s", r)
	}
	claims = map[string]any{"role": "unknown"}
	if r := mapClaimsToRole(claims, ac); r != "" {
		t.Fatalf("expected empty (no match), got %s", r)
	}
}
