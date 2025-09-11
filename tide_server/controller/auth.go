package controller

import (
	"errors"
	"log/slog"
	"net/http"

	"tide/common"
	"tide/tide_server/auth"
	"tide/tide_server/db"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func Login(c *gin.Context) {
	userManager.Login(c.Request, c.Writer)
}

func Logout(c *gin.Context) {
	userManager.Logout(c.Request, c.Writer)
}

func ListUser(c *gin.Context) {
	var condition, role = 1, auth.NormalUser
	if c.Query("application") == "true" {
		condition, role = 0, auth.DisabledUser
	}
	users, err := userManager.ListUsers(condition, role)
	if err != nil {
		slog.Error("Failed to list users", "error", err)
		return
	}
	c.JSON(http.StatusOK, users)
}

func UserInfo(c *gin.Context) {
	c.JSON(http.StatusOK, c.MustGet(contextKeyUserInfo))
}

func EditUser(c *gin.Context) {
	var (
		loginRole     = c.GetInt(contextKeyRole)
		loginUsername = c.GetString(contextKeyUsername)
		err           error
	)
	editMu.Lock()
	defer editMu.Unlock()
	if loginRole == auth.NormalUser {
		// can only update his own settings(email and password)
		var reqUser auth.UserBaseInfo
		if err = c.Bind(&reqUser); err != nil {
			return
		}
		reqUser.Username = loginUsername
		err = userManager.EditUserBaseInfo(reqUser)
		if err != nil {
			slog.Error("Failed to edit user base info", "username", loginUsername, "error", err)
			return
		}
	} else {
		var reqUser auth.User
		if err = c.Bind(&reqUser); err != nil {
			return
		}
		if reqUser.Username == "" {
			reqUser.Username = loginUsername
		}
		if reqUser.Username == loginUsername {
			// update himself
			err = userManager.EditUserBaseInfo(reqUser.UserBaseInfo)
			if err != nil {
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
					closeConnByUser(reqUser.Username, connTypeAny)
				} else if reqUserPre.Role != reqUser.Role {
					if reqUser.Role == auth.NormalUser {
						permissions, err := authorization.GetPermissions(reqUser.Username)
						if err != nil {
							slog.Error("Failed to get user permissions", "username", reqUser.Username, "error", err)
							return
						}
						handlePermissionChange(reqUser.Username, permissions)
					} else {
						handlePermissionChange(reqUser.Username, nil)
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
	_, _ = c.Writer.Write([]byte("ok"))
}

func DelUser(c *gin.Context) {
	var usernames []string
	if c.Bind(&usernames) != nil {
		return
	}
	for _, username := range usernames {
		user, err := userManager.GetUser(username)
		if err != nil {
			return
		}
		if user.Role <= auth.Admin { //normal user and admin can be deleted
			if err = userManager.DelUser(username); err != nil {
				slog.Error("Failed to delete user", "username", username, "error", err)
				return
			}
			closeConnByUser(username, connTypeAny)

			go mailDelUser(user.Username, user.Email)
		}
	}
	_, _ = c.Writer.Write([]byte("ok"))
}

func ApplyAccount(c *gin.Context) {
	var baseInfo auth.UserBaseInfo
	if err := c.Bind(&baseInfo); err != nil {
		return
	}
	err := userManager.AddUser(auth.User{UserBaseInfo: baseInfo, UserAuthority: auth.UserAuthority{Role: auth.DisabledUser}})
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
	_, _ = c.Writer.Write([]byte("ok"))
}

func PassApplication(c *gin.Context) {
	var usernames []string
	if c.Bind(&usernames) != nil {
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
	_, _ = c.Writer.Write([]byte("ok"))
}

func ListPermission(c *gin.Context) {
	var (
		err      error
		username string
		role     = c.GetInt(contextKeyRole)
	)
	if role == auth.NormalUser {
		username = c.GetString(contextKeyUsername)
	} else if role >= auth.Admin {
		username = c.Query("username")
	} else {
		return
	}
	var permissions = make(common.UUIDStringsMap)
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
	c.JSON(http.StatusOK, permissions)
}

type editPermissionStruct struct {
	Username    string                `json:"username" binding:"required"`
	Permissions common.UUIDStringsMap `json:"scopes"`
}

func EditPermission(c *gin.Context) {
	var params editPermissionStruct
	if c.Bind(&params) != nil {
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
	handlePermissionChange(params.Username, params.Permissions)
	_, _ = c.Writer.Write([]byte("ok"))
}

func ListUpstream(c *gin.Context) {
	upstreams, err := db.GetUpstreams()
	if err != nil {
		slog.Error("Failed to get upstreams list", "error", err)
		return
	}
	c.JSON(http.StatusOK, upstreams)
}

func EditUpstream(c *gin.Context) {
	var upstream db.Upstream
	if c.Bind(&upstream) != nil {
		return
	}
	editMu.Lock()
	defer editMu.Unlock()
	if err := db.EditUpstream(&upstream); err != nil {
		slog.Error("Failed to edit upstream", "upstream_id", upstream.Id, "error", err)
		return
	}
	if value, ok := recvConnections.Load(upstream.Id); ok {
		value.(*upstreamStorage).cancelF()
	}
	go startSync(upstream)
	_, _ = c.Writer.Write([]byte("ok"))
}

func DelUpstream(c *gin.Context) {
	var params struct {
		Id int `form:"id" binding:"required"`
	}
	if c.Bind(&params) != nil {
		return
	}

	editMu.Lock()
	defer editMu.Unlock()
	stationIds, err := db.DelUpstream(params.Id)
	if err != nil {
		slog.Error("Failed to delete upstream", "upstream_id", params.Id, "error", err)
		return
	}
	for _, stationId := range stationIds {
		Publish(configPubSub, SendMsgStruct{Type: kMsgDelUpstreamStation, Body: stationId}, nil)
	}
	if value, ok := recvConnections.LoadAndDelete(params.Id); ok {
		value.(*upstreamStorage).cancelF()
	}
	_, _ = c.Writer.Write([]byte("ok"))
}
