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
