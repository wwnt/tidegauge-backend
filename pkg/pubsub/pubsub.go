package pubsub

import (
	"context"
	"sync"
	"time"
)

// TopicSet holds a set of topics.
type TopicSet = map[any]struct{}

type Broker struct {
	subscriptions sync.Map
}

func NewBroker() *Broker {
	return &Broker{}
}

type Subscriber struct {
	Ch     chan any
	Cancel context.CancelFunc
}

func NewSubscriber(buffer int, cancelOnOverflow context.CancelFunc) *Subscriber {
	if cancelOnOverflow == nil {
		cancelOnOverflow = func() {}
	}
	return &Subscriber{
		Ch:     make(chan any, buffer),
		Cancel: cancelOnOverflow,
	}
}

// Subscribe adds or modifies a subscriber and its subscribed topics.
func (b *Broker) Subscribe(subscriber *Subscriber, subscribedTopics TopicSet) {
	b.subscriptions.Store(subscriber, subscribedTopics)
}

// RestrictTopics limits the set of topics for a specific subscriber.
func (b *Broker) RestrictTopics(subscriber *Subscriber, allowedTopics TopicSet) {
	if allowedTopics == nil {
		return
	}
	storedFilter, ok := b.subscriptions.Load(subscriber)
	if !ok {
		return
	}
	currentTopics, _ := storedFilter.(TopicSet)

	restrictedTopics := make(TopicSet)
	for topic := range currentTopics {
		if _, ok = allowedTopics[topic]; ok {
			restrictedTopics[topic] = struct{}{}
		}
	}
	if len(restrictedTopics) == 0 {
		b.Unsubscribe(subscriber)
		return
	}
	b.subscriptions.Store(subscriber, restrictedTopics)
}

// Unsubscribe deletes a subscriber.
func (b *Broker) Unsubscribe(subscriber *Subscriber) {
	b.subscriptions.Delete(subscriber)
}

// Publish sends a message to subscribers. If topic is specified, the message is
// only sent to subscribers that include that topic in their filter.
func (b *Broker) Publish(message any, topic any) {
	b.subscriptions.Range(func(subscriberKey, topicsValue any) bool {
		subscriber := subscriberKey.(*Subscriber)
		subscribedTopics, _ := topicsValue.(TopicSet)

		if topic != nil && subscribedTopics != nil {
			if _, ok := subscribedTopics[topic]; !ok {
				return true
			}
		}

		select {
		case subscriber.Ch <- message:
		default:
			b.Unsubscribe(subscriber)
			subscriber.Cancel()
		}
		return true
	})
}

type scheduledMessage struct {
	topic     any
	message   any
	publishAt time.Time
}

type DelayedBroker struct {
	mu             sync.Mutex
	broker         *Broker
	scheduledQueue []scheduledMessage
	wakeCh         chan struct{}
	delay          time.Duration
}

func NewDelayedBroker(broker *Broker, delay time.Duration) *DelayedBroker {
	delayedBroker := &DelayedBroker{
		broker: broker,
		wakeCh: make(chan struct{}),
		delay:  delay,
	}
	go delayedBroker.dispatchLoop()
	return delayedBroker
}

func (db *DelayedBroker) Publish(message any, topic any) {
	if db.delay == 0 {
		db.broker.Publish(message, topic)
		return
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if len(db.scheduledQueue) == 0 {
		db.wakeCh <- struct{}{}
	}
	db.scheduledQueue = append(db.scheduledQueue, scheduledMessage{
		topic:     topic,
		message:   message,
		publishAt: time.Now().Add(db.delay),
	})
}

func (db *DelayedBroker) dispatchLoop() {
	timer := time.NewTimer(100000 * time.Hour)
	for {
		db.mu.Lock()
		if len(db.scheduledQueue) == 0 {
			db.mu.Unlock()
			timer.Reset(100000 * time.Hour)
		} else {
			timer.Reset(db.scheduledQueue[0].publishAt.Sub(time.Now()))
			db.mu.Unlock()
		}

		select {
		case <-timer.C:
			db.mu.Lock()
			if len(db.scheduledQueue) == 0 {
				db.mu.Unlock()
				continue
			}
			nextMessage := db.scheduledQueue[0]
			db.scheduledQueue = db.scheduledQueue[1:]
			db.mu.Unlock()
			db.broker.Publish(nextMessage.message, nextMessage.topic)
		case <-db.wakeCh:
			if !timer.Stop() {
				<-timer.C
			}
		}
	}
}
