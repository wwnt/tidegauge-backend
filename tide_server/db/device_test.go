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

// 上游没带 specs（nil）的设备也必须能写入：devices.specs 是 NOT NULL 的 jsonb，
// nil 要落成 JSON null 而不是 SQL NULL。新建和已存在两种情况都要覆盖。
func (s *dbSuite) TestSyncDevice_NilSpecs() {
	fresh := Device{StationId: upstream1Station1.Id, Name: "device_without_specs"}
	got, err := SyncDevice(fresh)
	s.Require().NoError(err)
	s.EqualValues(1, got)

	existing := Device{StationId: upstream1Station1.Id, Name: upstream1Device1.Name}
	got, err = SyncDevice(existing)
	s.Require().NoError(err)
	s.EqualValues(0, got, "nil specs equals the stored JSON null, so nothing changes")

	devices, err := GetDevices(upstream1Station1.Id)
	s.Require().NoError(err)
	s.Len(devices, 2)
	for _, d := range devices {
		s.JSONEq("null", string(d.Specs))
	}
}

func (s *dbSuite) TestSyncDeviceRecord() {
	got, err := SyncDeviceRecord(upstream1DeviceRecord1)
	s.Require().NoError(err)
	s.EqualValues(0, got)
}
