package db

func (s *dbSuite) TestDelDevice() {
	got, err := DelDevice(station1.Id, device1.Name)
	s.Require().NoError(err)
	s.EqualValues(1, got)
}

func (s *dbSuite) TestEditDevice() {
	err := EditDevice(device1)
	s.Require().NoError(err)
}

func (s *dbSuite) TestEditDeviceRecord() {
	err := EditDeviceRecord(&deviceRecord1)
	s.Require().NoError(err)
}

func (s *dbSuite) TestGetDeviceRecords() {
	got, err := GetDeviceRecords()
	s.Require().NoError(err)
	// Query doesn't specify ordering.
	s.ElementsMatch([]DeviceRecord{deviceRecord1, upstream1DeviceRecord1}, got)
}

func (s *dbSuite) TestGetDevices() {
	got, err := GetDevices(station1.Id)
	s.Require().NoError(err)
	s.Equal([]Device{device1}, got)
}

func (s *dbSuite) TestSyncDevice() {
	got, err := SyncDevice(device1)
	s.Require().NoError(err)
	s.EqualValues(0, got)
}

func (s *dbSuite) TestSyncDeviceRecord() {
	got, err := SyncDeviceRecord(upstream1DeviceRecord1)
	s.Require().NoError(err)
	s.EqualValues(0, got)
}
