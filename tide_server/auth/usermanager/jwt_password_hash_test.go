package usermanager

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/alexedwards/argon2id"
	"golang.org/x/crypto/pbkdf2"
)

func TestVerifyPasswordHashArgon2(t *testing.T) {
	password := "s3cr3t"
	hash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		t.Fatalf("create argon2 hash: %v", err)
	}

	if !verifyPasswordHash(password, hash) {
		t.Fatalf("expected argon2 hash to verify")
	}
	if verifyPasswordHash("wrong-password", hash) {
		t.Fatalf("expected wrong password to fail")
	}
}

func TestVerifyPasswordHashPHCPBKDF2SHA256(t *testing.T) {
	password := "phc-pass"
	salt := []byte("salt-12345")
	iterations := 1000
	checksum := pbkdf2.Key([]byte(password), salt, iterations, sha256.Size, sha256.New)
	hash := fmt.Sprintf(
		"$pbkdf2-sha256$i=%d,l=%d$%s$%s",
		iterations,
		sha256.Size,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(checksum),
	)
	if !verifyPasswordHash(password, hash) {
		t.Fatalf("expected phc pbkdf2-sha256 hash to verify")
	}
	if verifyPasswordHash("wrong-password", hash) {
		t.Fatalf("expected wrong password to fail")
	}
}

func TestVerifyPasswordHashPHCPBKDF2SHA256WithoutKeyLen(t *testing.T) {
	password := "phc-pass-no-l"
	salt := []byte("salt-999")
	iterations := 2000
	checksum := pbkdf2.Key([]byte(password), salt, iterations, sha256.Size, sha256.New)
	hash := fmt.Sprintf(
		"$pbkdf2-sha256$i=%d$%s$%s",
		iterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(checksum),
	)
	if !verifyPasswordHash(password, hash) {
		t.Fatalf("expected phc pbkdf2-sha256 hash without l to verify")
	}
}

func TestVerifyPasswordHashPHCPBKDF2SHA256WithVersion(t *testing.T) {
	password := "phc-pass-v"
	salt := []byte("salt-ver")
	iterations := 3000
	checksum := pbkdf2.Key([]byte(password), salt, iterations, sha256.Size, sha256.New)
	hash := fmt.Sprintf(
		"$pbkdf2-sha256$v=0$i=%d,l=%d$%s$%s",
		iterations,
		sha256.Size,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(checksum),
	)
	if !verifyPasswordHash(password, hash) {
		t.Fatalf("expected phc pbkdf2-sha256 hash with version field to verify")
	}
}

func TestVerifyPasswordHashPHCPBKDF2SHA256InvalidParams(t *testing.T) {
	password := "phc-pass"
	salt := []byte("salt-abc")
	iterations := 1000
	checksum := pbkdf2.Key([]byte(password), salt, iterations, sha256.Size, sha256.New)
	encodedSalt := base64.RawStdEncoding.EncodeToString(salt)
	encodedChecksum := base64.RawStdEncoding.EncodeToString(checksum)

	cases := []string{
		fmt.Sprintf("$pbkdf2-sha256$rounds=%d$%s$%s", iterations, encodedSalt, encodedChecksum),
		fmt.Sprintf("$pbkdf2-sha256$i=%d,x=1$%s$%s", iterations, encodedSalt, encodedChecksum),
		fmt.Sprintf("$pbkdf2-sha256$i=%d,l=16$%s$%s", iterations, encodedSalt, encodedChecksum),
		fmt.Sprintf("$pbkdf2-sha256$i=0,l=%d$%s$%s", sha256.Size, encodedSalt, encodedChecksum),
		fmt.Sprintf("$pbkdf2-sha256$i=%d,l=%d$%s$%s", maxPBKDF2Iterations+1, sha256.Size, encodedSalt, encodedChecksum),
		fmt.Sprintf("$pbkdf2-sha256$i=%d,l=%d$%s$%s", iterations, maxPBKDF2KeyLen+1, encodedSalt, encodedChecksum),
		fmt.Sprintf("$pbkdf2-sha256$v=-1$i=%d,l=%d$%s$%s", iterations, sha256.Size, encodedSalt, encodedChecksum),
	}
	for _, hash := range cases {
		if verifyPasswordHash(password, hash) {
			t.Fatalf("expected invalid phc params to fail: %s", hash)
		}
	}
}
