package pubsub

import (
	"encoding/json"
	"go.uber.org/zap"
	"io"
	"sync"
	"time"
)

type BytesChan = chan []byte
type Subscriber = BytesChan

type TopicMap map[any]struct{}

type PubSub struct {
	subscribers  sync.Map
	writeTimeout time.Duration
}

func NewSubscriber(closeChan <-chan struct{}, conn io.WriteCloser) Subscriber {
	ch := make(BytesChan, 10)
	go func() {
		defer func() { _ = conn.Close() }()
		var (
			err   error
			bytes []byte
		)
		for {
			select {
			// For incoming server requests, the context is canceled when the client's connection closes,
			// the request is canceled (with HTTP/2), or when the ServeHTTP method returns.
			case <-closeChan:
				return
			case bytes = <-ch:
				if bytes == nil {
					return
				}
				_, err = conn.Write(bytes)
				if err != nil {
					return
				}
			}
		}
	}()
	return ch
}

func NewPubSub() *PubSub {
	p := new(PubSub)
	p.writeTimeout = time.Second
	return p
}

// SubscribeTopic add or modify subscriber
func (p *PubSub) SubscribeTopic(subscriber Subscriber, topics TopicMap) {
	p.subscribers.Store(subscriber, topics)
}

func (p *PubSub) LimitTopicScope(subscriber Subscriber, scope TopicMap) {
	if scope == nil {
		return
	}
	oldTopics, ok := p.subscribers.LoadAndDelete(subscriber)
	if !ok {
		return
	}
	var newTopics = make(TopicMap)
	for key := range oldTopics.(TopicMap) {
		if _, ok := scope[key]; ok {
			newTopics[key] = struct{}{}
		}
	}
	if len(newTopics) > 0 {
		p.subscribers.LoadOrStore(subscriber, newTopics)
	}
}

// Evict delete the subscriber
func (p *PubSub) Evict(subscriber Subscriber) {
	p.subscribers.Delete(subscriber)
}

// EvictAndClose delete the subscriber and close the subscriber
func (p *PubSub) EvictAndClose(subscriber Subscriber) {
	if _, loaded := p.subscribers.LoadAndDelete(subscriber); loaded {
		subscriber <- nil // make the subscriber exit the loop
	}
}

// Publish data to the subscribers that subscribed to the topic. If topic is nil, it will be sent to all subscribers.
func (p *PubSub) Publish(data any, topic any) (err error) {
	var mb []byte
	p.subscribers.Range(func(key, value any) bool {
		if topic != nil {
			if topics := value.(TopicMap); topics != nil { // make sure topics not nil
				if _, ok := topics[topic]; !ok {
					return true
				}
			}
		}
		if mb == nil {
			if mb, err = json.Marshal(data); err != nil {
				return false
			}
		}
		subscriber := key.(Subscriber)
		// If there is a write error, delete it from subscribers and close the subscriber to avoid sending to this subscriber in the next cycle
		select {
		case subscriber <- mb:
		default:
			p.EvictAndClose(subscriber)
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
