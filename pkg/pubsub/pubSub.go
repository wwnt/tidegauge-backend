package pubsub

import (
	"encoding/json"
	"io"
	"sync"
	"time"

	"go.uber.org/zap"
)

type BytesChan = chan []byte
type Subscriber = BytesChan

// TopicMap holds a set of topics
type TopicMap map[any]struct{}

type PubSub struct {
	mutex        sync.Mutex
	subscribers  map[Subscriber]TopicMap
	writeTimeout time.Duration
}

// NewSubscriber creates a new subscriber. When the closeChan signal is received, it closes the connection.
func NewSubscriber(closeChan <-chan struct{}, conn io.WriteCloser) Subscriber {
	ch := make(BytesChan, 10000000)
	go func() {
		defer func() { _ = conn.Close() }()
		var (
			err   error
			bytes []byte
		)
		for {
			select {
			// Exit when connection is closed or when the close signal is received.
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
	return &PubSub{
		subscribers:  make(map[Subscriber]TopicMap),
		writeTimeout: time.Second,
	}
}

// SubscribeTopic adds or modifies a subscriber and their subscribed topics
func (p *PubSub) SubscribeTopic(subscriber Subscriber, topics TopicMap) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.subscribers[subscriber] = topics
}

// LimitTopicScope limits the scope of topics for a specific subscriber
func (p *PubSub) LimitTopicScope(subscriber Subscriber, scope TopicMap) {
	if scope == nil {
		return
	}
	p.mutex.Lock()
	oldTopics, ok := p.subscribers[subscriber]
	if !ok {
		p.mutex.Unlock()
		return
	}
	// Remove the subscriber from the map first
	delete(p.subscribers, subscriber)
	p.mutex.Unlock()

	// Calculate the new topic intersection
	newTopics := make(TopicMap)
	for key := range oldTopics {
		if _, ok := scope[key]; ok {
			newTopics[key] = struct{}{}
		}
	}

	if len(newTopics) > 0 {
		p.mutex.Lock()
		p.subscribers[subscriber] = newTopics
		p.mutex.Unlock()
	}
}

// Evict deletes a subscriber
func (p *PubSub) Evict(subscriber Subscriber) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	delete(p.subscribers, subscriber)
}

// EvictAndClose deletes a subscriber and closes the subscriber's channel to notify it to exit
func (p *PubSub) EvictAndClose(subscriber Subscriber) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.evictAndClose(subscriber)
}
func (p *PubSub) evictAndClose(subscriber Subscriber) {
	delete(p.subscribers, subscriber)
	close(subscriber)
}

// Publish sends data to subscribers. If a topic is specified, the message is only sent to subscribers who are subscribed to that topic.
// If sending fails, the corresponding subscriber will be deleted and closed.
func (p *PubSub) Publish(data any, topic any) (err error) {
	var mb []byte
	p.mutex.Lock()
	defer p.mutex.Unlock()

	for subscriber, topics := range p.subscribers {
		// If a specific topic is passed, only send to subscribers subscribed to that topic
		if topic != nil {
			if topics != nil {
				if _, ok := topics[topic]; !ok {
					continue
				}
			}
		}
		// Serialize data the first time it is sent
		if mb == nil {
			mb, err = json.Marshal(data)
			if err != nil {
				return err
			}
		}
		// Non-blocking send, if the channel is blocked, evict and close the subscriber
		select {
		case subscriber <- mb:
		default:
			p.evictAndClose(subscriber)
		}
	}
	return nil
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
