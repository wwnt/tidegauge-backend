package db

func (s *dbSuite) TestDelUpstream() {
	got, err := DelUpstream(upstream1.Id)
	s.Require().NoError(err)
	s.Nil(got)
}

func (s *dbSuite) TestEditUpstream() {
	err := EditUpstream(&upstream1)
	s.Require().NoError(err)
}

// 修改时密码留空表示沿用已保存的密码，并把生效的密码回填给调用方。
func (s *dbSuite) TestEditUpstreamEmptyPasswordKeepsStoredOne() {
	edited := Upstream{Id: upstream1.Id, Username: "renamed", Password: "", Url: upstream1.Url}
	s.Require().NoError(EditUpstream(&edited))
	s.Equal(upstream1.Password, edited.Password, "the stored password must be handed back to the caller")

	got, err := GetUpstreams()
	s.Require().NoError(err)
	s.Contains(got, Upstream{Id: upstream1.Id, Username: "renamed", Password: upstream1.Password, Url: upstream1.Url})

	changed := Upstream{Id: upstream1.Id, Username: "renamed", Password: "new-secret", Url: upstream1.Url}
	s.Require().NoError(EditUpstream(&changed))
	got, err = GetUpstreams()
	s.Require().NoError(err)
	s.Contains(got, changed)
}

func (s *dbSuite) TestGetStationsByUpstreamId() {
	// GetStationsByUpstreamId does not return the Upstream bool field.
	tmp := upstream1Station1
	tmp.Upstream = false

	got, err := GetStationsByUpstreamId(upstream1.Id)
	s.Require().NoError(err)
	s.Equal([]Station{tmp}, got)
}

func (s *dbSuite) TestGetUpstreams() {
	got, err := GetUpstreams()
	s.Require().NoError(err)
	s.ElementsMatch([]Upstream{upstream1, upstream2}, got)
}

func (s *dbSuite) TestGetUpstreamsByStationId() {
	got, err := GetUpstreamsByStationId(upstream1Station1.Id)
	s.Require().NoError(err)
	s.ElementsMatch([]Upstream{upstream1, upstream2}, got)
}

func (s *dbSuite) TestIsUpstreamStation() {
	got, err := IsUpstreamStation(upstream1Station1.Id)
	s.Require().NoError(err)
	s.True(got)
}
