package syncv2station

import (
	"sync"

	"github.com/google/uuid"
)

type registry struct {
	conns sync.Map // map[uuid.UUID]*stationConn
}

func (r *registry) Load(stationID uuid.UUID) (*stationConn, bool) {
	v, ok := r.conns.Load(stationID)
	if !ok {
		return nil, false
	}
	c, _ := v.(*stationConn)
	return c, c != nil
}

func (r *registry) LoadOrStore(stationID uuid.UUID, conn *stationConn) (actual *stationConn, loaded bool) {
	v, loaded := r.conns.LoadOrStore(stationID, conn)
	if v == nil {
		return nil, loaded
	}
	c, _ := v.(*stationConn)
	return c, loaded
}

func (r *registry) Delete(stationID uuid.UUID) {
	r.conns.Delete(stationID)
}
