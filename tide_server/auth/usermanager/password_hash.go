package usermanager

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/alexedwards/argon2id"
	"golang.org/x/crypto/pbkdf2"
)

const (
	// Bound PBKDF2 work factors from database to avoid pathological verification costs.
	maxPBKDF2Iterations = 10_000_000
	maxPBKDF2KeyLen     = 1024
)

func createPasswordHash(password string) (string, error) {
	return argon2id.CreateHash(password, argon2id.DefaultParams)
}

func verifyPasswordHash(password, hash string) bool {
	switch {
	case strings.HasPrefix(hash, "$argon2"):
		ok, err := argon2id.ComparePasswordAndHash(password, hash)
		return err == nil && ok
	case strings.HasPrefix(hash, "$pbkdf2-sha256$"):
		return verifyPHCPBKDF2SHA256(password, hash)
	default:
		return false
	}
}

func verifyPHCPBKDF2SHA256(password, encodedHash string) bool {
	paramsPart, saltPart, checksumPart, ok := parsePHCPBKDF2SHA256Parts(encodedHash)
	if !ok {
		return false
	}
	params, ok := parsePHCParams(paramsPart)
	if !ok {
		return false
	}
	for key := range params {
		if key != "i" && key != "l" {
			return false
		}
	}
	iterationsText, ok := params["i"]
	if !ok {
		return false
	}
	iterations, err := strconv.Atoi(iterationsText)
	if err != nil || iterations <= 0 || iterations > maxPBKDF2Iterations {
		return false
	}
	dkLen := 0
	if lText, ok := params["l"]; ok {
		l, err := strconv.Atoi(lText)
		if err != nil || l <= 0 || l > maxPBKDF2KeyLen {
			return false
		}
		dkLen = l
	}

	salt, err := decodePHCBase64(saltPart)
	if err != nil || len(salt) == 0 {
		return false
	}
	checksum, err := decodePHCBase64(checksumPart)
	if err != nil || len(checksum) == 0 {
		return false
	}
	if dkLen == 0 {
		dkLen = len(checksum)
	}
	if dkLen > maxPBKDF2KeyLen || dkLen != len(checksum) {
		return false
	}
	derived := pbkdf2.Key([]byte(password), salt, iterations, dkLen, sha256.New)
	return subtle.ConstantTimeCompare(derived, checksum) == 1
}

func parsePHCPBKDF2SHA256Parts(encodedHash string) (params string, salt string, checksum string, ok bool) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 5 && len(parts) != 6 {
		return "", "", "", false
	}
	if parts[0] != "" || parts[1] != "pbkdf2-sha256" {
		return "", "", "", false
	}
	paramsIdx := 2
	if len(parts) == 6 {
		if !strings.HasPrefix(parts[2], "v=") {
			return "", "", "", false
		}
		version, err := strconv.Atoi(strings.TrimPrefix(parts[2], "v="))
		if err != nil || version < 0 {
			return "", "", "", false
		}
		paramsIdx = 3
	}
	if parts[paramsIdx] == "" || parts[paramsIdx+1] == "" || parts[paramsIdx+2] == "" {
		return "", "", "", false
	}
	return parts[paramsIdx], parts[paramsIdx+1], parts[paramsIdx+2], true
}

func parsePHCParams(s string) (map[string]string, bool) {
	if s == "" {
		return nil, false
	}
	params := make(map[string]string)
	for _, item := range strings.Split(s, ",") {
		kv := strings.SplitN(item, "=", 2)
		if len(kv) != 2 || kv[0] == "" || kv[1] == "" {
			return nil, false
		}
		if _, exists := params[kv[0]]; exists {
			return nil, false
		}
		params[kv[0]] = kv[1]
	}
	return params, true
}

func decodePHCBase64(s string) ([]byte, error) {
	if decoded, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return decoded, nil
	}
	return base64.StdEncoding.DecodeString(s)
}
