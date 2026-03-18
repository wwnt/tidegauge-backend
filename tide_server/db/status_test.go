package db

import (
	"time"

	"tide/common"
)

func (s *dbSuite) TestGetItemStatusLogs() {
	got, err := GetItemStatusLogs(station1.Id, 0)
	s.Require().NoError(err)
	s.Equal(station1StatusLogs, got)
}

func (s *dbSuite) TestGetLatestStatusLogRowId() {
	got, err := GetLatestStatusLogRowId(station1.Id)
	s.Require().NoError(err)
	s.Equal(station1StatusLogs[len(station1StatusLogs)-1].RowId, got)
}

func (s *dbSuite) TestPagedItemStatusLogs() {
	got, err := PagedItemStatusLogs(1, 10)
	s.Require().NoError(err)

	want := pagedItemStatusLogStruct{Total: len(station1StatusLogs), Data: []common.StationIdItemStatusStruct{
		{StationId: station1.Id, ItemStatusStruct: station1StatusLogs[1].ItemStatusStruct},
		{StationId: station1.Id, ItemStatusStruct: station1StatusLogs[0].ItemStatusStruct},
	}}
	s.Equal(want, got)
}

func (s *dbSuite) TestSaveItemStatusLog() {
	got, err := SaveItemStatusLog(station1.Id, station1StatusLogs[len(station1StatusLogs)-1].RowId+1, item1.Name, common.Normal, time.Now())
	s.Require().NoError(err)
	s.EqualValues(1, got)
}

func (s *dbSuite) TestUpdateAndSaveStatusLog() {
	got, err := UpdateAndSaveStatusLog(station1.Id, station1StatusLogs[len(station1StatusLogs)-1].RowId+1, item1.Name, common.Normal, time.Now())
	s.Require().NoError(err)
	s.EqualValues(1, got)
}

func (s *dbSuite) TestUpdateItemStatus() {
	got, err := UpdateItemStatus(station1.Id, item1.Name, common.NoStatus, time.Now())
	s.Require().NoError(err)
	s.EqualValues(1, got)
}

func (s *dbSuite) TestUpdateStationStatus() {
	got, err := UpdateStationStatus(station1.Id, common.Disconnected, time.Now())
	s.Require().NoError(err)
	s.EqualValues(1, got)
}
