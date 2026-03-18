package db

import (
	"time"

	"tide/pkg/custype"

	"github.com/google/uuid"
)

func (s *dbSuite) TestGetDataHistory() {
	got, err := GetDataHistory(station1.Id, item1.Name, 0, 3)
	s.Require().NoError(err)
	s.Equal(data, got)
}

func (s *dbSuite) TestGetItemsLatest() {
	itemsLatest := map[string]custype.UnixMs{item1.Name: 0}
	err := GetItemsLatest(station1.Id, itemsLatest)
	s.Require().NoError(err)
}

func (s *dbSuite) TestGetLatestDataTime() {
	got, err := GetLatestDataTime(station1.Id, item1.Name)
	s.Require().NoError(err)
	s.Equal(data[1].Millisecond, got)
}

func (s *dbSuite) TestSaveDataHistory_InsertAlreadyExists() {
	got, err := SaveDataHistory(station1.Id, item1.Name, data[1].Value, data[1].Millisecond.ToTime())
	s.Require().NoError(err)
	s.EqualValues(0, got)
}

func (s *dbSuite) TestGetDataHistory_NoRowsDoesNotError() {
	got, err := GetDataHistory(uuid.New(), item1.Name, 0, 0)
	s.Require().NoError(err)
	s.Empty(got)
}

func (s *dbSuite) TestSaveDataHistory_NewPoint() {
	got, err := SaveDataHistory(station1.Id, item1.Name, 0.2, time.UnixMilli(3))
	s.Require().NoError(err)
	s.EqualValues(1, got)
}
