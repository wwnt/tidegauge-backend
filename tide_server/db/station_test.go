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
	// upstream1Station1 同时挂在 upstream1 和 upstream2 下：摘掉第一个上游时站点保留，
	// 摘掉最后一个上游时才真正删除。
	got, err := DelUpstreamStation(upstream1.Id, upstream1Station1.Id)
	s.Require().NoError(err)
	s.EqualValues(0, got)

	got, err = DelUpstreamStation(upstream2.Id, upstream1Station1.Id)
	s.Require().NoError(err)
	s.EqualValues(1, got)
}

// 上游同步路径不得删除本地站点：stationId 来自上游发来的消息，
// 带着本地站点 UUID 的删除事件必须是空操作。
func (s *dbSuite) TestDelUpstreamStation_LeavesLocalStationAlone() {
	got, err := DelUpstreamStation(upstream1.Id, station1.Id)
	s.Require().NoError(err)
	s.EqualValues(0, got)

	id, err := GetLocalStationIdByIdentifier(station1.Identifier)
	s.Require().NoError(err)
	s.Equal(station1.Id, id)
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

// 上游发来一个与本地站点同 id 的站点时，本地站点必须原样保留：不变成上游站点，
// 也不与该上游建立关联（否则删除上游时它会被当成上游站点处理）。
func (s *dbSuite) TestSyncStation_DoesNotTakeOverLocalStation() {
	incoming := station1
	incoming.Location = json.RawMessage(`{"changed":true}`)
	got, err := SyncStation(upstream1.Id, incoming)
	s.Require().NoError(err)
	s.EqualValues(0, got)

	up, err := IsUpstreamStation(station1.Id)
	s.Require().NoError(err)
	s.False(up, "a live local station must stay local")
	id, err := GetLocalStationIdByIdentifier(station1.Identifier)
	s.Require().NoError(err)
	s.Equal(station1.Id, id)

	linked, err := GetStationsByUpstreamId(upstream1.Id)
	s.Require().NoError(err)
	for _, st := range linked {
		s.NotEqual(station1.Id, st.Id, "a local station must not be linked to an upstream")
	}
}

// 被软删除的本地站点可以被上游的同 id 站点接管（原有行为）。
func (s *dbSuite) TestSyncStation_RevivesSoftDeletedStationAsUpstream() {
	_, err := DelLocalStation(station1.Id)
	s.Require().NoError(err)

	got, err := SyncStation(upstream1.Id, station1)
	s.Require().NoError(err)
	s.EqualValues(1, got)
	up, err := IsUpstreamStation(station1.Id)
	s.Require().NoError(err)
	s.True(up)
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
