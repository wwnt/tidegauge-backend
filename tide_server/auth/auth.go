package auth

import (
	"errors"
	"github.com/google/uuid"
	"net/http"
)

const (
	DisabledUser = iota - 1
	NormalUser
	Admin
	SuperAdmin
)

type (
	UserBaseInfo struct {
		Username string `json:"username" binding:"lowercase"`
		Password string `json:"password,omitempty"`
		Email    string `json:"email" binding:"omitempty,email"`
	}
	UserAuthority struct {
		Role       int  `json:"role"`
		LiveCamera bool `json:"live_camera"`
	}
	User struct {
		UserBaseInfo
		UserAuthority
	}
)

var (
	ErrUserEmpty     = errors.New("the username or password is empty")
	ErrUserNotFound  = errors.New("user not found")
	ErrUserDuplicate = errors.New("user duplicate")
)

// UserManager is the interface that can manage users.
type UserManager interface {
	CheckUserPwd(username, password string) bool

	Login(r *http.Request, w http.ResponseWriter)

	Logout(r *http.Request, w http.ResponseWriter)
	// GetLoginUser get the current login user
	GetLoginUser(r *http.Request) (string, error)
	// AddUser adds a new user.
	AddUser(user User) error
	// DelUser removes a user by username.
	DelUser(username string) error

	EditUserBaseInfo(user UserBaseInfo) error

	EditUser(user User) error

	ListUsers(condition int, role int) ([]User, error)

	GetUser(username string) (User, error)
}

type Permission interface {
	CheckPermission(username string, stationId uuid.UUID, itemName string) bool
	GetPermissions(string) (map[uuid.UUID][]string, error)
	EditPermission(string, map[uuid.UUID][]string) error
	CheckCameraStatusPermission(username string, stationId uuid.UUID, name string) bool
	GetCameraStatusPermissions(string) (map[uuid.UUID][]string, error)
	EditCameraStatusPermission(string, map[uuid.UUID][]string) error
}
