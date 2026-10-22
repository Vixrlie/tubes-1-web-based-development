package session_test

import (
	"regexp"
	"testing"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
)

func TestNewToken(t *testing.T) {
	re := regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`) // 32 byte -> 43 karakter base64url tanpa padding

	t.Run("format base64url 43 karakter", func(t *testing.T) {
		tok, err := session.NewToken()
		if err != nil {
			t.Fatalf("NewToken: %v", err)
		}
		if !re.MatchString(tok) {
			t.Fatalf("token %q tidak sesuai format", tok)
		}
	})

	t.Run("seribu token tidak ada yang kembar", func(t *testing.T) {
		seen := map[string]bool{}
		for i := 0; i < 1000; i++ {
			tok, err := session.NewToken()
			if err != nil {
				t.Fatalf("NewToken: %v", err)
			}
			if seen[tok] {
				t.Fatalf("token kembar pada iterasi %d", i)
			}
			seen[tok] = true
		}
	})
}

func TestHashToken(t *testing.T) {
	hexRe := regexp.MustCompile(`^[0-9a-f]{64}$`)

	h := session.HashToken("abc")
	if !hexRe.MatchString(h) {
		t.Fatalf("hash %q bukan 64 karakter hex (SHA-256)", h)
	}
	if h != session.HashToken("abc") {
		t.Error("hash harus deterministik")
	}
	if h == session.HashToken("abd") {
		t.Error("input berbeda harus menghasilkan hash berbeda")
	}
	if h == "abc" {
		t.Error("hash tidak boleh sama dengan token")
	}
}
