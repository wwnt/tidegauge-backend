package controller

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"testing"

	"tide/common"
	"tide/tide_server/auth"
	"tide/tide_server/db"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
)

func Test_handleSyncServerConn(t *testing.T) {
	truncateDB(t)

	username := "user01"
	err := userManager.AddUser(user01)
	if err != nil && !errors.Is(err, auth.ErrUserDuplicate) {
		require.NoError(t, err)
	}
	initialPermissions := common.UUIDStringsMap{
		station1.Id: {"location1_air_humidity"},
	}
	updatedPermissions := common.UUIDStringsMap{
		station1.Id: {"location1_air_visibility"},
	}
	require.NoError(t, authorization.EditPermission(username, initialPermissions))

	conn1, conn2 := net.Pipe() // conn3: sync server   , conn4: sync client

	go func() {
		defer func() { _ = conn1.Close() }()
		handleSyncServerConn(context.Background(), conn1, username, initialPermissions)
	}()

	session, err := yamux.Client(conn2, nil)
	if err != nil {
		slog.Error("Failed to create yamux client session in test", "error", err)
		return
	}
	defer func() {
		slog.Debug("Sync client closed in test")
		_ = session.Close()
	}()

	stream1, err := session.Open()
	if err != nil {
		slog.Error("Failed to open stream1 in test", "error", err)
		return
	}
	stream2, err := session.Open()
	if err != nil {
		slog.Error("Failed to open stream2 in test", "error", err)
		return
	}

	//if !fullSyncConfigClient(stream2, upstream) {
	//	return
	//}
	//_ = stream2.Close()
	decoder := json.NewDecoder(stream2)
	encoder := json.NewEncoder(stream2)
	var stationsFull []db.StationFullInfo
	err = decoder.Decode(&stationsFull)
	require.NoError(t, err)

	var deviceRecords []db.DeviceRecord
	err = decoder.Decode(&deviceRecords)
	require.NoError(t, err)

	err = encoder.Encode(map[uuid.UUID]int64{station1.Id: 0})
	require.NoError(t, err)

	var missStatusLogs map[uuid.UUID][]common.RowIdItemStatusStruct
	err = decoder.Decode(&missStatusLogs)
	require.NoError(t, err)
	slog.Debug("Miss status logs in test", "data", missStatusLogs)

	_ = stream2.Close()

	defer func() { _ = session.Close() }()
	stream3, err := session.Open()
	require.NoError(t, err)

	stream4, err := session.Open()
	require.NoError(t, err)

	encoder = json.NewEncoder(stream4)
	decoder = json.NewDecoder(stream4)

	var firstPermissions common.UUIDStringsMap
	err = decoder.Decode(&firstPermissions)
	require.NoError(t, err)
	require.Equal(t, initialPermissions, firstPermissions)

	var stationsItemsLatest = make(map[uuid.UUID]common.StringMsecMap)
	err = encoder.Encode(stationsItemsLatest)
	require.NoError(t, err)

	var stationsMissData map[uuid.UUID]map[string][]common.DataTimeStruct
	err = decoder.Decode(&stationsMissData)
	require.NoError(t, err)
	slog.Debug("Stations miss data in test", "data", stationsMissData)
	_ = stream4.Close()

	require.NoError(t, authorization.EditPermission(username, updatedPermissions))
	hub.UpdatePermissions(username, updatedPermissions)
	_ = stream3.Close()

	stream3, err = session.Open()
	require.NoError(t, err)
	stream4, err = session.Open()
	require.NoError(t, err)
	encoder = json.NewEncoder(stream4)
	decoder = json.NewDecoder(stream4)

	var refreshedPermissions common.UUIDStringsMap
	err = decoder.Decode(&refreshedPermissions)
	require.NoError(t, err)
	require.Equal(t, updatedPermissions, refreshedPermissions)

	err = encoder.Encode(stationsItemsLatest)
	require.NoError(t, err)
	err = decoder.Decode(&stationsMissData)
	require.NoError(t, err)
	_ = stream4.Close()
	_ = stream3.Close()

	_ = stream1.Close()
}

type permissionLoaderErrStub struct {
	getPermissionsErr error
}

func (s *permissionLoaderErrStub) CheckPermission(string, uuid.UUID, string) bool {
	return false
}

func (s *permissionLoaderErrStub) GetPermissions(string) (map[uuid.UUID][]string, error) {
	return nil, s.getPermissionsErr
}

func (s *permissionLoaderErrStub) EditPermission(string, map[uuid.UUID][]string) error {
	return nil
}

func (s *permissionLoaderErrStub) CheckCameraStatusPermission(string, uuid.UUID, string) bool {
	return false
}

func (s *permissionLoaderErrStub) GetCameraStatusPermissions(string) (map[uuid.UUID][]string, error) {
	return nil, nil
}

func (s *permissionLoaderErrStub) EditCameraStatusPermission(string, map[uuid.UUID][]string) error {
	return nil
}

func Test_currentSyncDataScope_FailClosedOnPermissionError(t *testing.T) {
	truncateDB(t)

	err := userManager.AddUser(user01)
	if err != nil && !errors.Is(err, auth.ErrUserDuplicate) {
		require.NoError(t, err)
	}

	prevAuthorization := authorization
	authorization = &permissionLoaderErrStub{getPermissionsErr: errors.New("permission query failed")}
	t.Cleanup(func() {
		authorization = prevAuthorization
	})

	permissions, topics := currentSyncDataScope(user01.Username)
	require.NotNil(t, permissions)
	require.Empty(t, permissions)
	require.NotNil(t, topics)
	require.Empty(t, topics)
}
