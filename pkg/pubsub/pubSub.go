package pubsub

import (
	"encoding/json"
	"go.uber.org/zap"
	"io"
	"sync"
	"time"
)

type PubConn interface {
	io.WriteCloser
	SetWriteDeadline(time.Time) error
}

type TopicMap map[any]struct{}

type PubSub struct {
	subscribers  sync.Map
	writeTimeout time.Duration
}

func NewPubSub() *PubSub {
	p := new(PubSub)
	p.writeTimeout = time.Second
	return p
}

// SubscribeTopic add or modify subscriber
func (p *PubSub) SubscribeTopic(conn PubConn, topic TopicMap) {
	p.subscribers.Store(conn, topic)
}

func (p *PubSub) LimitTopicScope(conn PubConn, scope TopicMap) {
	if scope == nil {
		return
	}
	oldTopic, ok := p.subscribers.LoadAndDelete(conn)
	if !ok {
		return
	}
	var newTopic = make(TopicMap)
	for key := range oldTopic.(TopicMap) {
		if _, ok := scope[key]; ok {
			newTopic[key] = struct{}{}
		}
	}
	if len(newTopic) > 0 {
		p.subscribers.LoadOrStore(conn, newTopic)
	}
}

// Evict delete the subscriber
func (p *PubSub) Evict(conn PubConn) {
	p.subscribers.Delete(conn)
}

// EvictAndClose delete the subscriber and close the connection
func (p *PubSub) EvictAndClose(conn PubConn) {
	if _, loaded := p.subscribers.LoadAndDelete(conn); loaded {
		_ = conn.Close()
	}
}

// Publish data to the connections that subscribed to the subKey. If subKey is nil, it will be sent to all connections.
func (p *PubSub) Publish(data any, subKey any) (err error) {
	var mb []byte
	p.subscribers.Range(func(conn, topic any) bool {
		if subKey != nil {
			if s := topic.(TopicMap); s != nil { //make sure topic not empty
				if _, ok := s[subKey]; !ok {
					return true
				}
			}
		}
		if mb == nil {
			if mb, err = json.Marshal(data); err != nil {
				return false
			}
			mb = append(mb, '\n')
		}
		// If there is a write error, delete it from subscribers and close the connection to avoid sending to this channel in the next cycle
		if err = conn.(PubConn).SetWriteDeadline(time.Now().Add(p.writeTimeout)); err != nil {
			p.EvictAndClose(conn.(PubConn))
		} else {
			if _, err = conn.(PubConn).Write(mb); err != nil {
				p.EvictAndClose(conn.(PubConn))
			}
		}
		return true
	})
	return err
}

type delayPubEntry struct {
	subKey any
	data   any
	t      time.Time
}

type DelayPublish struct {
	mu sync.Mutex
	*PubSub
	delayPubEntries []delayPubEntry
	blockInEmptyCh  chan struct{}
	delay           time.Duration
	logger          *zap.Logger
}

func NewDelayPublish(ps *PubSub, delay time.Duration, logger *zap.Logger) *DelayPublish {
	p := &DelayPublish{
		PubSub:         ps,
		blockInEmptyCh: make(chan struct{}),
		delay:          delay,
		logger:         logger,
	}
	go p.run()
	return p
}

func (p *DelayPublish) DelayPublish(data any, subKey any) {
	if p.delay == 0 {
		err := p.Publish(data, subKey)
		if err != nil {
			p.logger.WithOptions(zap.AddCallerSkip(1)).DPanic("publish", zap.Error(err))
		}
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.delayPubEntries) == 0 {
		p.blockInEmptyCh <- struct{}{}
	}
	p.delayPubEntries = append(p.delayPubEntries, delayPubEntry{subKey: subKey, data: data, t: time.Now().Add(p.delay)})
}

func (p *DelayPublish) run() {
	var timer = time.NewTimer(100000 * time.Hour)
	var err error
	for {
		if len(p.delayPubEntries) == 0 {
			timer.Reset(100000 * time.Hour)
		} else {
			timer.Reset(p.delayPubEntries[0].t.Sub(time.Now()))
		}
		select {
		case <-timer.C:
			err = p.Publish(p.delayPubEntries[0].subKey, p.delayPubEntries[0].data)
			if err != nil {
				p.logger.DPanic("publish", zap.Error(err))
			}
			p.mu.Lock()
			p.delayPubEntries = p.delayPubEntries[1:]
			p.mu.Unlock()
		case <-p.blockInEmptyCh:
			if !timer.Stop() {
				<-timer.C
			}
		}
	}
}
