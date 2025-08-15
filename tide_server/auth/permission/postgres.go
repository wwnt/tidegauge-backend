package permission

import (
	"database/sql"
	"github.com/google/uuid"
	"tide/common"
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
	if err == nil && rowsAffected > 0 {
		return true
	} else {
		return false
	}
}

func (p *PG) GetPermissions(username string) (map[uuid.UUID][]string, error) {
	rows, err := p.Query("select station_id, item_name from permissions_item_data where username=$1", username)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var (
		stationId uuid.UUID
		itemName  string
		scope     = make(common.UUIDStringsMap)
	)
	for rows.Next() {
		err = rows.Scan(&stationId, &itemName)
		if err != nil {
			return nil, err
		}
		scope[stationId] = append(scope[stationId], itemName)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return scope, nil
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
	if err == nil && rowsAffected > 0 {
		return true
	} else {
		return false
	}
}

func (p *PG) GetCameraStatusPermissions(username string) (map[uuid.UUID][]string, error) {
	rows, err := p.Query("select station_id, camera_name from permissions_camera_status where username=$1", username)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var (
		stationId  uuid.UUID
		cameraName string
		scopes     = make(map[uuid.UUID][]string)
	)
	for rows.Next() {
		err = rows.Scan(&stationId, &cameraName)
		if err != nil {
			return nil, err
		}
		scopes[stationId] = append(scopes[stationId], cameraName)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return scopes, nil
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
