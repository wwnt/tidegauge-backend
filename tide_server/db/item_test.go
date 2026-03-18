package db

import (
	"tide/common"

	"github.com/google/uuid"
)

func (s *dbSuite) TestDelItem() {
	got, err := DelItem(station1.Id, item1.Name)
	s.Require().NoError(err)
	s.EqualValues(1, got)
}

func (s *dbSuite) TestEditItem() {
	err := EditItem(item1)
	s.Require().NoError(err)
}

func (s *dbSuite) TestGetAvailableItems() {
	got, err := GetAvailableItems()
	s.Require().NoError(err)
	s.Equal([]common.StationItemStruct{{StationId: station1.Id, ItemName: item1.Name}}, got)
}

func (s *dbSuite) TestGetItems() {
	got, err := GetItems(station1.Id)
	s.Require().NoError(err)
	s.Equal([]Item{item1}, got)
}

func (s *dbSuite) TestMakeSureTableExist_NewTable() {
	err := MakeSureTableExist("test_table_name")
	s.Require().NoError(err)
}

func (s *dbSuite) TestRemoveAllAvailable() {
	err := RemoveAvailableByUpstreamId(upstream1.Id)
	s.Require().NoError(err)
}

func (s *dbSuite) TestSyncItem() {
	got, err := SyncItem(upstream1Item1)
	s.Require().NoError(err)
	s.EqualValues(0, got)
}

func (s *dbSuite) TestUpdateAvailableItems() {
	got, err := UpdateAvailableItems(upstream1.Id, map[uuid.UUID][]string{upstream1Station1.Id: {item1.Name}})
	s.Require().NoError(err)
	s.EqualValues(1, got)
}
