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
		w.WriteHeader(http.StatusInternalServerError)
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

// EditUser 修改用户。非管理员只能改自己的邮箱和密码；管理员可以改自己（同样只改
// 邮箱/密码），也可以新建或修改其他非超级管理员的账号。
//
// 失败路径都返回明确的状态码——以前直接 return，客户端拿到的是 200 空响应，
// 分不清成功还是失败。
func EditUser(w http.ResponseWriter, r *http.Request) {
	var (
		loginRole     = requestRole(r)
		loginUsername = requestUsername(r)
		err           error
	)
	editMu.Lock()
	defer editMu.Unlock()

	// 必须用 "< Admin" 而不是 "== NormalUser"：后者会让任何非 0 角色（包括待审批账号的 -1）
	// 进入下面的管理员分支。
	if loginRole < auth.Admin {
		// can only update his own settings(email and password)
		var reqUser auth.UserBaseInfo
		if !readJSONOrBadRequest(w, r, &reqUser) {
			return
		}
		reqUser.Username = loginUsername
		if err = userManager.EditUserBaseInfo(reqUser); err != nil {
			slog.Error("Failed to edit user base info", "username", loginUsername, "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeOK(w)
		return
	}

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
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeOK(w)
		return
	}

	// update others
	if reqUser.Role >= auth.SuperAdmin { // cannot be set to superAdmin
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if reqUser.Role < auth.DisabledUser {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	reqUserPre, err := userManager.GetUser(reqUser.Username)
	switch {
	case err == nil:
		//can not edit super admin
		if reqUserPre.Role >= auth.SuperAdmin {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if err = userManager.EditUser(reqUser); err != nil {
			slog.Error("Failed to edit user", "username", reqUser.Username, "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if reqUser.Password != "" || reqUserPre.Role != reqUser.Role {
			// 密码或角色变了：断开该用户的全部连接，让它们按新的身份重新建立。
			// 已打开的 WebSocket/同步连接是按旧角色放行的，只收窄订阅范围不够——
			// 被降级的管理员仍可在旧连接上重新订阅任意数据。
			hub.DisconnectUser(reqUser.Username, connTypeAny)
		}
	case errors.Is(err, auth.ErrUserNotFound):
		if err = userManager.AddUser(reqUser); err != nil {
			slog.Error("Failed to add new user", "username", reqUser.Username, "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	default:
		slog.Error("Failed to get user for edit", "username", reqUser.Username, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
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
			if errors.Is(err, auth.ErrUserNotFound) {
				continue // 已经不存在，继续处理列表里的其余用户
			}
			slog.Error("Failed to get user for delete", "username", username, "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if user.Role <= auth.Admin {
			if err = userManager.DelUser(username); err != nil {
				slog.Error("Failed to delete user", "username", username, "error", err)
				w.WriteHeader(http.StatusInternalServerError)
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
		switch {
		case errors.Is(err, auth.ErrUserDuplicate):
			w.WriteHeader(http.StatusConflict)
		case errors.Is(err, auth.ErrUserEmpty):
			w.WriteHeader(http.StatusBadRequest)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
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
			if errors.Is(err, auth.ErrUserNotFound) {
				continue
			}
			slog.Error("Failed to get user for application approval", "username", username, "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if user.Role == auth.DisabledUser {
			user.Role = auth.NormalUser
			if err = userManager.EditUser(user); err != nil {
				slog.Error("Failed to pass user application", "username", username, "error", err)
				w.WriteHeader(http.StatusInternalServerError)
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
	if role >= auth.Admin {
		username = r.URL.Query().Get("username")
	} else {
		username = requestUsername(r)
	}

	permissions := make(common.UUIDStringsMap)
	if role >= auth.Admin && username == "" {
		items, err := db.GetItems(uuid.Nil)
		if err != nil {
			slog.Error("Failed to get all items for admin permissions", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		for _, item := range items {
			permissions[item.StationId] = append(permissions[item.StationId], item.Name)
		}
	} else {
		permissions, err = authorization.GetPermissions(username)
		if err != nil {
			slog.Error("Failed to get user permissions", "username", username, "error", err)
			w.WriteHeader(http.StatusInternalServerError)
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
	// "scopes" 缺省或为 null 表示"没有任何权限"。必须归一化成空表：
	// hub.UpdatePermissions 把 nil 解释为"不限制"（管理员语义），直接传 nil 会让
	// 权限被清空的用户在已打开的连接上继续收到全部数据。
	if params.Permissions == nil {
		params.Permissions = common.UUIDStringsMap{}
	}

	editMu.Lock()
	defer editMu.Unlock()

	dstUser, err := userManager.GetUser(params.Username)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		slog.Error("Failed to get user for permission edit", "username", params.Username, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// can not edit admins permission
	if dstUser.Role >= auth.Admin {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if err = authorization.EditPermission(params.Username, params.Permissions); err != nil {
		slog.Error("Failed to edit user permissions", "username", params.Username, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	hub.UpdatePermissions(params.Username, params.Permissions)
	writeOK(w)
}

// ListUpstream 列出上游配置。密码不返回：它是本服务器在别的服务器上的账号凭据，
// 任何管理员会话都不应该能把它读出来。
func ListUpstream(w http.ResponseWriter, _ *http.Request) {
	upstreams, err := db.GetUpstreams()
	if err != nil {
		slog.Error("Failed to get upstreams list", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	for i := range upstreams {
		upstreams[i].Password = ""
	}
	writeJSON(w, http.StatusOK, upstreams)
}

// EditUpstream 新建或修改上游。修改时密码留空表示沿用已保存的密码
// （ListUpstream 不再返回密码，前端原样回传时这里不能把它清空）。
func EditUpstream(w http.ResponseWriter, r *http.Request) {
	var upstream db.Upstream
	if !readJSONOrBadRequest(w, r, &upstream) {
		return
	}
	if upstream.Url == "" || upstream.Username == "" || (upstream.Id == 0 && upstream.Password == "") {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	editMu.Lock()
	defer editMu.Unlock()
	if err := db.EditUpstream(&upstream); err != nil {
		slog.Error("Failed to edit upstream", "upstream_id", upstream.Id, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
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
		w.WriteHeader(http.StatusInternalServerError)
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
