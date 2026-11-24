package permission

import (
	"database/sql"

	"github.com/google/uuid"
)

type PG struct {
	*sql.DB
}

func NewPostgres(db *sql.DB) *PG {
	return &PG{db}
}

func (p *PG) CheckPermission(username string, stationId uuid.UUID, itemName string) bool {
	res, err := p.Exec("select from permissions_item_data where username=$1 and station_id=$2 and item_name=$3", username, stationId, itemName)
	if err != nil {
		return false
	}
	rowsAffected, err := res.RowsAffected()
	return err == nil && rowsAffected > 0
}

func (p *PG) GetPermissions(username string) (map[uuid.UUID][]string, error) {
	return p.queryScopes("select station_id, item_name from permissions_item_data where username=$1", username)
}

// queryScopes runs a query returning (station_id, name) rows and groups the
// names by station.
func (p *PG) queryScopes(query string, username string) (map[uuid.UUID][]string, error) {
	rows, err := p.Query(query, username)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	scopes := make(map[uuid.UUID][]string)
	for rows.Next() {
		var (
			stationId uuid.UUID
			name      string
		)
		if err = rows.Scan(&stationId, &name); err != nil {
			return nil, err
		}
		scopes[stationId] = append(scopes[stationId], name)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return scopes, nil
}

func (p *PG) EditPermission(username string, scopes map[uuid.UUID][]string) error {
	tx, err := p.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec("delete from permissions_item_data where username=$1", username)
	if err != nil {
		return err
	}

	for stationId, items := range scopes {
		for _, item := range items {
			_, err = tx.Exec("insert into permissions_item_data(username, station_id, item_name) VALUES ($1,$2,$3)", username, stationId, item)
			if err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (p *PG) CheckCameraStatusPermission(username string, stationId uuid.UUID, name string) bool {
	res, err := p.Exec("select from permissions_camera_status where username=$1 and station_id=$2 and camera_name=$3", username, stationId, name)
	if err != nil {
		return false
	}
	rowsAffected, err := res.RowsAffected()
	return err == nil && rowsAffected > 0
}

func (p *PG) GetCameraStatusPermissions(username string) (map[uuid.UUID][]string, error) {
	return p.queryScopes("select station_id, camera_name from permissions_camera_status where username=$1", username)
}

func (p *PG) EditCameraStatusPermission(username string, scopes map[uuid.UUID][]string) error {
	tx, err := p.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec("delete from permissions_camera_status where username=$1", username)
	if err != nil {
		return err
	}

	for stationId, cameraNames := range scopes {
		for _, item := range cameraNames {
			_, err = tx.Exec("insert into permissions_camera_status(username, station_id, camera_name) VALUES ($1,$2,$3)", username, stationId, item)
			if err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
