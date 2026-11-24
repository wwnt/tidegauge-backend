package usermanager

import (
	"database/sql"
	"errors"

	"tide/tide_server/auth"
)

// listUsersFromDB queries users table and returns matching users according to condition.
// condition: -1 => role <= role, 0 => role == role, 1 => role >= role
func listUsersFromDB(db *sql.DB, condition int, role int) ([]auth.User, error) {
	var op string
	switch condition {
	case -1:
		op = "<="
	case 0:
		op = "="
	case 1:
		op = ">="
	default:
		return nil, nil
	}
	rows, err := db.Query("select username, role, email, live_camera from users where role "+op+" $1", role)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var users []auth.User
	for rows.Next() {
		var user auth.User
		if err = rows.Scan(&user.Username, &user.Role, &user.Email, &user.LiveCamera); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

// getUserFromDB queries a single user by username and maps sql.ErrNoRows to auth.ErrUserNotFound.
func getUserFromDB(db *sql.DB, username string) (auth.User, error) {
	var user auth.User
	err := db.QueryRow("select username, role, email, live_camera from users where username=$1", username).Scan(&user.Username, &user.Role, &user.Email, &user.LiveCamera)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return user, auth.ErrUserNotFound
		}
	}
	return user, err
}
