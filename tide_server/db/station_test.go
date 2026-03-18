package db

import (
	"encoding/json"

	"github.com/google/uuid"
)

func (s *dbSuite) TestDelLocalStation() {
	got, err := DelLocalStation(station1.Id)
	s.Require().NoError(err)
	s.EqualValues(1, got)
}

func (s *dbSuite) TestDelUpstreamStation() {
	got, err := DelUpstreamStation(upstream1.Id, station1.Id)
	s.Require().NoError(err)
	s.EqualValues(1, got)
}

func (s *dbSuite) TestEditStation() {
	err := EditStation(&station1)
	s.Require().NoError(err)
}

func (s *dbSuite) TestEditStationNoSync() {
	err := EditStationNotSync(station1.Id, station1.IpAddr)
	s.Require().NoError(err)
}

func (s *dbSuite) TestGetLocalStationIdByIdentifier() {
	got, err := GetLocalStationIdByIdentifier(station1.Identifier)
	s.Require().NoError(err)
	s.Equal(station1.Id, got)
}

func (s *dbSuite) TestGetStations() {
	got, err := GetStations()
	s.Require().NoError(err)
	// Query doesn't specify ordering.
	s.ElementsMatch([]Station{station1, upstream1Station1}, got)
}

func (s *dbSuite) TestGetStationsFullInfo() {
	got, err := GetStationsFullInfo()
	s.Require().NoError(err)
	// Query doesn't specify ordering.
	s.ElementsMatch([]StationFullInfo{{
		Station: station1,
		Items:   []Item{item1},
		Devices: []Device{device1},
	}, {
		Station: upstream1Station1,
		Items:   []Item{upstream1Item1},
		Devices: []Device{upstream1Device1},
	}}, got)
}

func (s *dbSuite) TestSyncStation() {
	got, err := SyncStation(upstream1.Id, upstream1Station1)
	s.Require().NoError(err)
	s.EqualValues(0, got)
}

func (s *dbSuite) TestSyncStationNoEdit() {
	got, err := SyncStationCannotEdit(upstream1Station1.Id, upstream1Station1.Cameras)
	s.Require().NoError(err)
	s.EqualValues(0, got)
}

func (s *dbSuite) TestSyncStationCannotEdit_UpdatesCameras() {
	newCameras := json.RawMessage(`["camera1","camera2"]`)
	got, err := SyncStationCannotEdit(upstream1Station1.Id, newCameras)
	s.Require().NoError(err)
	s.EqualValues(1, got)
}

func (s *dbSuite) TestSyncStationCannotEdit_NoChange() {
	got, err := SyncStationCannotEdit(upstream1Station1.Id, upstream1Station1.Cameras)
	s.Require().NoError(err)
	s.EqualValues(0, got)
}

func (s *dbSuite) TestDelUpstreamStation_NoChange() {
	got, err := DelUpstreamStation(upstream1.Id, uuid.New())
	// Deleting non-existent join row should succeed and delete 0 stations.
	s.Require().NoError(err)
	s.EqualValues(0, got)
}
