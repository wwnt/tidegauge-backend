package usermanager

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"tide/tide_server/auth"

	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// JwtManager implements auth.UserManager using local DB + JWT tokens.
type JwtManager struct {
	db         *sql.DB
	signingKey []byte
	issuer     string
	// token TTL
	expire time.Duration
}

// NewJwt creates a new JwtManager. signingKey must be a secret string.
func NewJwt(db *sql.DB, signingKey []byte, issuer string, expire time.Duration) *JwtManager {
	return &JwtManager{db: db, signingKey: signingKey, issuer: issuer, expire: expire}
}

// CheckUserPwd checks username/password against stored Argon2 PHC hash in users.password column.
func (j *JwtManager) CheckUserPwd(username, password string) bool {
	if username == "" || password == "" {
		return false
	}
	var hash string
	err := j.db.QueryRow("select password_hash from users where username=$1", username).Scan(&hash)
	if err != nil {
		return false
	}
	if hash == "" {
		return false
	}
	ok, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return false
	}
	return ok
}

// Login reads username/password from form and returns a JSON token similar to Keycloak's JWT structure.
func (j *JwtManager) Login(r *http.Request, w http.ResponseWriter) {
	username := r.PostFormValue("username")
	password := r.PostFormValue("password")
	if !j.CheckUserPwd(username, password) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	// create token
	now := time.Now()
	exp := now.Add(j.expire)
	claims := jwt.RegisteredClaims{
		Subject:   username,
		Issuer:    j.issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(j.signingKey)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// Return JSON compatible with gocloak.JWT fields used elsewhere (access_token etc.)
	resp := map[string]any{
		"access_token":       signed,
		"token_type":         "Bearer",
		"expires_in":         int(j.expire.Seconds()),
		"refresh_expires_in": 0,
		"refresh_token":      "",
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// Logout currently has no server-side token revocation; respond with 204.
func (j *JwtManager) Logout(_ *http.Request, w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// GetLoginUser extracts token from Authorization header (Bearer), cookie "token", or query param "token" and validates it.
func (j *JwtManager) GetLoginUser(r *http.Request) (string, error) {
	var tokenStr string
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, BearerPrefix) {
		tokenStr = authHeader[BearerPrefixLen:]
	} else if c, err := r.Cookie("token"); err == nil && c.Value != "" {
		tokenStr = c.Value
	} else if t := r.URL.Query().Get("token"); t != "" {
		tokenStr = t
	} else {
		return "", nil
	}
	if tokenStr == "" {
		return "", nil
	}
	// parse
	parsed, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{}, func(token *jwt.Token) (any, error) {
		// ensure method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return j.signingKey, nil
	})
	if err != nil {
		return "", nil
	}
	if claims, ok := parsed.Claims.(*jwt.RegisteredClaims); ok && parsed.Valid {
		return claims.Subject, nil
	}
	return "", nil
}

// AddUser inserts the user into the DB and stores an PHC password hash.
func (j *JwtManager) AddUser(user auth.User) error {
	if user.Username == "" || user.Password == "" {
		return auth.ErrUserEmpty
	}
	user.Username = strings.ToLower(user.Username)
	user.Email = strings.ToLower(user.Email)
	pwHash, err := argon2id.CreateHash(user.Password, argon2id.DefaultParams)
	if err != nil {
		return err
	}
	tx, err := j.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// store password hash and metadata
	_, err = tx.Exec("insert into users(username, password_hash, role, email, live_camera) values ($1,$2,$3,$4,$5)", user.Username, pwHash, user.Role, user.Email, user.LiveCamera)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { //ERROR: duplicate key value violates unique constraint "users_username_uindex" (SQLSTATE 23505)
			return auth.ErrUserDuplicate
		}
		return err
	}
	return tx.Commit()
}

// EditUserBaseInfo updates email and optionally password.
func (j *JwtManager) EditUserBaseInfo(user auth.UserBaseInfo) error {
	if user.Username == "" {
		return auth.ErrUserEmpty
	}
	if user.Password == "" {
		res, err := j.db.Exec("update users set email=$2 where username=$1", user.Username, user.Email)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return auth.ErrUserNotFound
		}
		return nil
	}
	pwHash, err := argon2id.CreateHash(user.Password, argon2id.DefaultParams)
	if err != nil {
		return err
	}
	res, err := j.db.Exec("update users set email=$2, password_hash=$3 where username=$1", user.Username, user.Email, pwHash)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return auth.ErrUserNotFound
	}
	return nil
}

// EditUser updates role/email/live_camera and optionally password.
func (j *JwtManager) EditUser(user auth.User) error {
	if user.Username == "" {
		return auth.ErrUserEmpty
	}
	tx, err := j.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec("update users set role=$2, email=$3, live_camera=$4 where username=$1", user.Username, user.Role, user.Email, user.LiveCamera)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return auth.ErrUserNotFound
	}
	if user.Password != "" {
		pwHash, err := argon2id.CreateHash(user.Password, argon2id.DefaultParams)
		if err != nil {
			return err
		}
		_, err = tx.Exec("update users set password_hash=$2 where username=$1", user.Username, pwHash)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListUsers lists users by condition as in Keycloak implementation.
func (j *JwtManager) ListUsers(condition int, role int) (users []auth.User, err error) {
	return listUsersFromDB(j.db, condition, role)
}

// GetUser returns stored user metadata.
func (j *JwtManager) GetUser(username string) (auth.User, error) {
	return getUserFromDB(j.db, username)
}

// DelUser removes user row from database.
func (j *JwtManager) DelUser(username string) error {
	res, err := j.db.Exec("delete from users where username=$1", username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return auth.ErrUserNotFound
	}
	return nil
}
