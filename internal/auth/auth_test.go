package auth

import "testing"

func TestPasswordHashAndVerify(t *testing.T) {
	s := NewService("test-secret")
	hash, err := s.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	if !s.VerifyPassword(hash, "password123") {
		t.Fatal("verify should succeed for correct password")
	}
	if s.VerifyPassword(hash, "wrong") {
		t.Fatal("verify should fail for wrong password")
	}
}

func TestPasswordTooShort(t *testing.T) {
	s := NewService("test-secret")
	if _, err := s.HashPassword("abc"); err == nil {
		t.Fatal("should reject short password")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	s := NewService("test-secret")
	token, err := s.GenerateToken("usr_1", "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	claims, err := s.ParseToken(token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.UserID != "usr_1" || claims.Username != "alice" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestTokenInvalidSecret(t *testing.T) {
	a := NewService("secret-a")
	b := NewService("secret-b")
	token, _ := a.GenerateToken("u", "n")
	if _, err := b.ParseToken(token); err == nil {
		t.Fatal("token signed with another secret should fail")
	}
}

func TestGenerateID(t *testing.T) {
	id := GenerateID("usr")
	if len(id) < 5 {
		t.Fatalf("id too short: %s", id)
	}
	if id[:4] != "usr_" {
		t.Fatalf("wrong prefix: %s", id)
	}
}
