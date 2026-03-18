package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"tide/tide_server/auth"

	"github.com/stretchr/testify/require"
)

func TestValidateAdminMiddleware_ForbiddenForNonAdmin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = withRequestUser(req, "user01", auth.User{
		UserBaseInfo:  auth.UserBaseInfo{Username: "user01"},
		UserAuthority: auth.UserAuthority{Role: auth.NormalUser},
	})

	w := httptest.NewRecorder()
	h := validateAdminMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestValidateLiveSnapshotMiddleware_ForbiddenForNormalUserWithoutLiveCamera(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = withRequestUser(req, "user01", auth.User{
		UserBaseInfo:  auth.UserBaseInfo{Username: "user01"},
		UserAuthority: auth.UserAuthority{Role: auth.NormalUser, LiveCamera: false},
	})

	w := httptest.NewRecorder()
	h := validateLiveSnapshotMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestValidateLiveSnapshotMiddleware_AllowsNormalUserWithLiveCamera(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = withRequestUser(req, "user01", auth.User{
		UserBaseInfo:  auth.UserBaseInfo{Username: "user01"},
		UserAuthority: auth.UserAuthority{Role: auth.NormalUser, LiveCamera: true},
	})

	w := httptest.NewRecorder()
	h := validateLiveSnapshotMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}
