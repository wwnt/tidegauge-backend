package controller

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"tide/common"
	"tide/tide_server/auth"
	"tide/tide_server/db"

	"github.com/google/uuid"
)

func Login(w http.ResponseWriter, r *http.Request) {
	userManager.Login(r, w)
}

func Logout(w http.ResponseWriter, r *http.Request) {
	userManager.Logout(r, w)
}

func ListUser(w http.ResponseWriter, r *http.Request) {
	var condition, role = 1, auth.NormalUser
	if r.URL.Query().Get("application") == "true" {
		condition, role = 0, auth.DisabledUser
	}
	users, err := userManager.ListUsers(condition, role)
	if err != nil {
		slog.Error("Failed to list users", "error", err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func UserInfo(w http.ResponseWriter, r *http.Request) {
	user, ok := requestUserInfo(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func EditUser(w http.ResponseWriter, r *http.Request) {
	var (
		loginRole     = requestRole(r)
		loginUsername = requestUsername(r)
		err           error
	)
	editMu.Lock()
	defer editMu.Unlock()

	if loginRole == auth.NormalUser {
		// can only update his own settings(email and password)
		var reqUser auth.UserBaseInfo
		if !readJSONOrBadRequest(w, r, &reqUser) {
			return
		}
		reqUser.Username = loginUsername
		if err = userManager.EditUserBaseInfo(reqUser); err != nil {
			slog.Error("Failed to edit user base info", "username", loginUsername, "error", err)
			return
		}
	} else {
		var reqUser auth.User
		if !readJSONOrBadRequest(w, r, &reqUser) {
			return
		}
		if reqUser.Username == "" {
			reqUser.Username = loginUsername
		}
		if reqUser.Username == loginUsername {
			// update himself
			if err = userManager.EditUserBaseInfo(reqUser.UserBaseInfo); err != nil {
				slog.Error("Failed to edit user base info for self", "username", loginUsername, "error", err)
				return
			}
		} else {
			// update others
			if reqUser.Role >= auth.SuperAdmin { // cannot be set to superAdmin
				return
			}
			reqUserPre, err := userManager.GetUser(reqUser.Username)
			if err == nil {
				//can not edit super admin
				if reqUserPre.Role >= auth.SuperAdmin {
					return
				}
				if err = userManager.EditUser(reqUser); err != nil {
					slog.Error("Failed to edit user", "username", reqUser.Username, "error", err)
					return
				}
				if reqUser.Password != "" || reqUser.Role == auth.DisabledUser {
					// password changed or disable login
					hub.DisconnectUser(reqUser.Username, connTypeAny)
				} else if reqUserPre.Role != reqUser.Role {
					if reqUser.Role == auth.NormalUser {
						permissions, err := authorization.GetPermissions(reqUser.Username)
						if err != nil {
							slog.Error("Failed to get user permissions", "username", reqUser.Username, "error", err)
							return
						}
						hub.UpdatePermissions(reqUser.Username, permissions)
					} else {
						hub.UpdatePermissions(reqUser.Username, nil)
					}
				}
			} else if errors.Is(err, auth.ErrUserNotFound) {
				if err = userManager.AddUser(reqUser); err != nil {
					slog.Error("Failed to add new user", "username", reqUser.Username, "error", err)
					return
				}
			} else {
				slog.Error("Failed to get user for edit", "username", reqUser.Username, "error", err)
				return
			}
		}
	}
	writeOK(w)
}

func DelUser(w http.ResponseWriter, r *http.Request) {
	var usernames []string
	if !readJSONOrBadRequest(w, r, &usernames) {
		return
	}
	for _, username := range usernames {
		user, err := userManager.GetUser(username)
		if err != nil {
			return
		}
		if user.Role <= auth.Admin {
			if err = userManager.DelUser(username); err != nil {
				slog.Error("Failed to delete user", "username", username, "error", err)
				return
			}
			hub.DisconnectUser(username, connTypeAny)
			go mailDelUser(user.Username, user.Email)
		}
	}
	writeOK(w)
}

func ApplyAccount(w http.ResponseWriter, r *http.Request) {
	var baseInfo auth.UserBaseInfo
	if !readJSONOrBadRequest(w, r, &baseInfo) {
		return
	}
	err := userManager.AddUser(auth.User{
		UserBaseInfo:  baseInfo,
		UserAuthority: auth.UserAuthority{Role: auth.DisabledUser},
	})
	if err != nil {
		slog.Warn("Failed to apply account", "username", baseInfo.Username, "error", err)
		return
	}
	go func() {
		// send mail to all admins
		users, err := userManager.ListUsers(1, auth.Admin)
		if err != nil {
			slog.Error("Failed to list admin users for notification", "error", err)
			return
		}
		var to []string
		for _, user := range users {
			if user.Email == "" {
				continue
			}
			to = append(to, user.Email)
		}
		if err = SendMail(to, "Have a new account application"); err != nil {
			slog.Error("Failed to send notification email", "error", err)
		}
	}()
	writeOK(w)
}

func PassApplication(w http.ResponseWriter, r *http.Request) {
	var usernames []string
	if !readJSONOrBadRequest(w, r, &usernames) {
		return
	}
	editMu.Lock()
	defer editMu.Unlock()
	for _, username := range usernames {
		user, err := userManager.GetUser(username)
		if err != nil {
			return
		}
		if user.Role == auth.DisabledUser {
			user.Role = auth.NormalUser
			if err = userManager.EditUser(user); err != nil {
				slog.Error("Failed to pass user application", "username", username, "error", err)
				return
			}
			go func() {
				if err := SendMail([]string{user.Email}, "Account application is successful"); err != nil {
					slog.Warn("Failed to send application success email", "username", user.Username, "error", err)
				}
			}()
		}
	}
	writeOK(w)
}

func ListPermission(w http.ResponseWriter, r *http.Request) {
	var (
		err      error
		username string
		role     = requestRole(r)
	)
	if role == auth.NormalUser {
		username = requestUsername(r)
	} else if role >= auth.Admin {
		username = r.URL.Query().Get("username")
	} else {
		return
	}

	permissions := make(common.UUIDStringsMap)
	if role >= auth.Admin && username == "" {
		items, err := db.GetItems(uuid.Nil)
		if err != nil {
			slog.Error("Failed to get all items for admin permissions", "error", err)
			return
		}
		for _, item := range items {
			permissions[item.StationId] = append(permissions[item.StationId], item.Name)
		}
	} else {
		permissions, err = authorization.GetPermissions(username)
		if err != nil {
			slog.Error("Failed to get user permissions", "username", username, "error", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, permissions)
}

type editPermissionStruct struct {
	Username    string                `json:"username"`
	Permissions common.UUIDStringsMap `json:"scopes"`
}

func EditPermission(w http.ResponseWriter, r *http.Request) {
	var params editPermissionStruct
	if !readJSONOrBadRequest(w, r, &params) {
		return
	}
	if params.Username == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	editMu.Lock()
	defer editMu.Unlock()

	dstUser, err := userManager.GetUser(params.Username)
	if err != nil {
		slog.Error("Failed to get user for permission edit", "username", params.Username, "error", err)
		return
	}
	// can not edit admins permission
	if dstUser.Role >= auth.Admin {
		return
	}
	if err = authorization.EditPermission(params.Username, params.Permissions); err != nil {
		slog.Error("Failed to edit user permissions", "username", params.Username, "error", err)
		return
	}
	hub.UpdatePermissions(params.Username, params.Permissions)
	writeOK(w)
}

func ListUpstream(w http.ResponseWriter, _ *http.Request) {
	upstreams, err := db.GetUpstreams()
	if err != nil {
		slog.Error("Failed to get upstreams list", "error", err)
		return
	}
	writeJSON(w, http.StatusOK, upstreams)
}

func EditUpstream(w http.ResponseWriter, r *http.Request) {
	var upstream db.Upstream
	if !readJSONOrBadRequest(w, r, &upstream) {
		return
	}
	editMu.Lock()
	defer editMu.Unlock()
	if err := db.EditUpstream(&upstream); err != nil {
		slog.Error("Failed to edit upstream", "upstream_id", upstream.Id, "error", err)
		return
	}
	if value, ok := recvConnections.Load(upstream.Id); ok {
		value.(*upstreamSyncState).cancel()
	}
	go startSync(upstream)
	writeOK(w)
}

func DelUpstream(w http.ResponseWriter, r *http.Request) {
	var id int

	_ = r.ParseForm()
	if raw := r.Form.Get("id"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		id = parsed
	} else {
		var params struct {
			Id int `json:"id"`
		}
		if !readJSONOrBadRequest(w, r, &params) {
			return
		}
		id = params.Id
	}

	if id == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	editMu.Lock()
	defer editMu.Unlock()
	stationIds, err := db.DelUpstream(id)
	if err != nil {
		slog.Error("Failed to delete upstream", "upstream_id", id, "error", err)
		return
	}
	for _, stationId := range stationIds {
		hub.Publish(BrokerConfig, SendMsgStruct{Type: kMsgDelUpstreamStation, Body: stationId}, nil)
	}
	if value, ok := recvConnections.LoadAndDelete(id); ok {
		value.(*upstreamSyncState).cancel()
	}
	writeOK(w)
}
