package usermanager

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/Nerzal/gocloak/v13"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
	"net/url"
	"strings"
	"tide/tide_server/auth"
)

type Keycloak struct {
	token                          *gocloak.JWT
	db                             *sql.DB
	client                         *gocloak.GoCloak
	basePath                       string
	masterUsername, masterPassword string
	realm, clientId, clientSecret  string
}

func (k *Keycloak) CheckUserPwd(username, password string) bool {
	ctx := context.Background()
	_, err := k.client.Login(ctx, k.clientId, k.clientSecret, k.realm, username, password)
	if err == nil {
		return true
	} else {
		return false
	}
}

func (k *Keycloak) Login(r *http.Request, w http.ResponseWriter) {
	ctx := context.Background()
	jwt, err := k.client.Login(ctx, k.clientId, k.clientSecret, k.realm, r.PostFormValue("username"), r.PostFormValue("password"))
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		_ = encoder.Encode(jwt)
	} else {
		w.WriteHeader(http.StatusUnauthorized)
	}
}

func (k *Keycloak) Logout(r *http.Request, w http.ResponseWriter) {
	ctx := context.Background()
	err := k.client.Logout(ctx, k.clientId, k.clientSecret, k.realm, r.PostFormValue("refresh_token"))
	if err == nil {
		w.WriteHeader(http.StatusNoContent)
	}
}

const (
	BearerPrefix    = "Bearer "
	BearerPrefixLen = len(BearerPrefix)
)

func (k *Keycloak) GetLoginUser(r *http.Request) (string, error) {
	var token string
	if token = r.Header.Get("Authorization"); strings.HasPrefix(token, BearerPrefix) {
		token = token[BearerPrefixLen:]
	} else if token = r.URL.Query().Get("token"); token != "" {
	} else {
		return "", nil
	}

	var values = make(url.Values)
	values.Set("token", token)
	values.Set("client_id", k.clientId)
	values.Set("client_secret", k.clientSecret)
	resp, err := http.PostForm(k.basePath+"/realms/"+k.realm+"/protocol/openid-connect/token/introspect", values)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var introspect struct {
		Active   bool   `json:"active"`
		Username string `json:"preferred_username"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&introspect); err != nil {
		return "", err
	}
	if !introspect.Active {
		return "", nil
	}
	return introspect.Username, nil
}

func (k *Keycloak) ListUsers(condition int, role int) (users []auth.User, err error) {
	var rows *sql.Rows
	switch condition {
	case -1:
		rows, err = k.db.Query("select username, role, email, live_camera from users where role <= $1", role)
	case 0:
		rows, err = k.db.Query("select username, role, email, live_camera from users where role = $1", role)
	case 1:
		rows, err = k.db.Query("select username, role, email, live_camera from users where role >= $1", role)
	}
	if err != nil {
		return nil, err
	}
	var user auth.User
	for rows.Next() {
		err = rows.Scan(&user.Username, &user.Role, &user.Email, &user.LiveCamera)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}
func (k *Keycloak) getKcUserByUsername(ctx context.Context, username string) (gocloak.User, error) {
	var exact = true
	kcUsers, err := k.client.GetUsers(ctx, k.token.AccessToken, k.realm, gocloak.GetUsersParams{Username: &username, Exact: &exact})
	if err != nil {
		return gocloak.User{}, err
	} else {
		for _, kcUser := range kcUsers {
			if *kcUser.Username == username {
				return *kcUser, nil
			}
		}
		return gocloak.User{}, auth.ErrUserNotFound
	}
}
func (k *Keycloak) GetUser(username string) (auth.User, error) {
	var user auth.User
	err := k.db.QueryRow("select username, role, email, live_camera from users where username=$1", username).Scan(&user.Username, &user.Role, &user.Email, &user.LiveCamera)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return user, auth.ErrUserNotFound
		}
	}
	return user, err
}

func (k *Keycloak) AddUser(user auth.User) error {
	if user.Username == "" || user.Password == "" {
		return auth.ErrUserEmpty
	}
	// Username and email can only be in lowercase
	user.Username = strings.ToLower(user.Username)
	user.Email = strings.ToLower(user.Email)
	tx, err := k.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	//If the Insert fails in the database, it will return directly
	_, err = tx.Exec("insert into users(username, role, email, live_camera) values ($1,$2,$3,$4)", user.Username, user.Role, user.Email, user.LiveCamera)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { //ERROR: duplicate key value violates unique constraint "users_username_uindex" (SQLSTATE 23505)
			return auth.ErrUserDuplicate
		}
		return err
	}

	ctx := context.Background()
	k.token, err = k.client.LoginAdmin(ctx, k.masterUsername, k.masterPassword, "master")
	if err != nil {
		return err
	}

	var enable bool
	if user.Role >= auth.NormalUser {
		enable = true
	}
	kcUser := gocloak.User{
		Username: &user.Username,
		Enabled:  &enable,
	}

	userUUID, err := k.client.CreateUser(ctx, k.token.AccessToken, k.realm, kcUser)
	if err != nil {
		var apiErr *gocloak.APIError
		if !errors.As(err, &apiErr) || apiErr.Code != http.StatusConflict { // User exists with same username
			return err
		}
		if kcUser, err = k.getKcUserByUsername(ctx, user.Username); err != nil {
			return err
		}
		userUUID = *kcUser.ID
		// If there is already a user with the same name, then update
		if *kcUser.Enabled != enable {
			*kcUser.Enabled = enable
			if err = k.client.UpdateUser(ctx, k.token.AccessToken, k.realm, kcUser); err != nil {
				return err
			}
		}
	}
	if err = k.client.SetPassword(ctx, k.token.AccessToken, userUUID, k.realm, user.Password, false); err != nil {
		return err
	}
	return tx.Commit()
}

// EditUserBaseInfo edit user basic info and password
func (k *Keycloak) EditUserBaseInfo(user auth.UserBaseInfo) error {
	if user.Username == "" {
		return auth.ErrUserEmpty
	}
	res, err := k.db.Exec("update users set email=$2 where username=$1", user.Username, user.Email)
	if err != nil {
		return err
	} else if n, _ := res.RowsAffected(); n == 0 {
		return auth.ErrUserNotFound
	}
	// change password
	if user.Password != "" {
		ctx := context.Background()
		k.token, err = k.client.LoginAdmin(ctx, k.masterUsername, k.masterPassword, "master")
		if err != nil {
			return err
		}
		kcUser, err := k.getKcUserByUsername(ctx, user.Username)
		if err != nil {
			if errors.Is(err, auth.ErrUserNotFound) { //almost never happens
				_, _ = k.db.Exec("delete from users where username=$1", user.Username)
				return err
			}
			return err
		}
		if err = k.client.SetPassword(ctx, k.token.AccessToken, *kcUser.ID, k.realm, user.Password, false); err != nil {
			return err
		}
	}
	return err
}

func (k *Keycloak) EditUser(user auth.User) error {
	if user.Username == "" {
		return auth.ErrUserEmpty
	}
	tx, err := k.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec("update users set role=$2, email=$3, live_camera=$4 where username=$1", user.Username, user.Role, user.Email, user.LiveCamera)
	if err != nil {
		return err
	} else if n, _ := res.RowsAffected(); n == 0 {
		return auth.ErrUserNotFound
	}
	// change password
	ctx := context.Background()
	k.token, err = k.client.LoginAdmin(ctx, k.masterUsername, k.masterPassword, "master")
	if err != nil {
		return err
	}
	kcUser, err := k.getKcUserByUsername(ctx, user.Username)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			_, err = tx.Exec("delete from users where username=$1", user.Username)
			if err != nil {
				return err
			}
			_ = tx.Commit()
		}
		return err
	}
	var enable bool
	if user.Role >= auth.NormalUser {
		enable = true
	}
	// If this user is found, then update
	if *kcUser.Enabled != enable {
		*kcUser.Enabled = enable
		if err = k.client.UpdateUser(ctx, k.token.AccessToken, k.realm, kcUser); err != nil {
			return err
		}
	}
	if user.Password != "" {
		if err = k.client.SetPassword(ctx, k.token.AccessToken, *kcUser.ID, k.realm, user.Password, false); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (k *Keycloak) DelUser(username string) (err error) {
	tx, err := k.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec("delete from users where username=$1", username)
	if err != nil {
		return err
	} else if n, _ := res.RowsAffected(); n == 0 {
		return auth.ErrUserNotFound
	}

	ctx := context.Background()
	k.token, err = k.client.LoginAdmin(ctx, k.masterUsername, k.masterPassword, "master")
	if err != nil {
		return err
	}
	kcUser, err := k.getKcUserByUsername(ctx, username)
	if err != nil {
		if !errors.Is(err, auth.ErrUserNotFound) {
			return err //Error finding user
		}
	} else {
		//Found a user
		err = k.client.DeleteUser(ctx, k.token.AccessToken, k.realm, *kcUser.ID)
		if err != nil {
			var apiErr *gocloak.APIError
			if !errors.As(err, &apiErr) || apiErr.Code != http.StatusNotFound {
				return err
			}
		}
	}
	return tx.Commit()
}

func NewKeycloak(db *sql.DB, basePath, masterUsername, masterPassword, realm, clientId, clientSecret string) *Keycloak {
	return &Keycloak{
		db:             db,
		client:         gocloak.NewClient(basePath, gocloak.SetAuthRealms("realms"), gocloak.SetAuthAdminRealms("admin/realms")),
		basePath:       basePath,
		masterUsername: masterUsername,
		masterPassword: masterPassword,
		realm:          realm,
		clientId:       clientId,
		clientSecret:   clientSecret,
	}
}
