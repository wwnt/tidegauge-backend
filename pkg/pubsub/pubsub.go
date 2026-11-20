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
		// 容量为 1：唤醒信号只表示"队列可能非空"，积压一个就够，发送方永不阻塞。
		wakeCh: make(chan struct{}, 1),
		delay:  delay,
	}
	go delayedBroker.dispatchLoop()
	return delayedBroker
}

func (db *DelayedBroker) Publish(message any, topic any) {
	if db.delay <= 0 {
		db.broker.Publish(message, topic)
		return
	}
	db.mu.Lock()
	wasEmpty := len(db.scheduledQueue) == 0
	db.scheduledQueue = append(db.scheduledQueue, scheduledMessage{
		topic:     topic,
		message:   message,
		publishAt: time.Now().Add(db.delay),
	})
	db.mu.Unlock()

	if wasEmpty {
		// 必须在释放 mu 之后、且以非阻塞方式唤醒：dispatchLoop 取走队首后要重新拿 mu，
		// 若这里持锁阻塞在发送上，两边会互相等待（旧实现的死锁）。
		select {
		case db.wakeCh <- struct{}{}:
		default:
		}
	}
}

// dispatchLoop 按入队顺序在各自的 publishAt 时刻转发消息。所有消息的延迟相同且按时间
// 顺序入队，所以队首的到期时间不会因为后续入队而提前，等待队首期间无需被唤醒。
func (db *DelayedBroker) dispatchLoop() {
	for {
		db.mu.Lock()
		if len(db.scheduledQueue) == 0 {
			db.mu.Unlock()
			// 可能读到一个过期的唤醒信号（队列已被取空），回到循环顶部重新检查即可。
			<-db.wakeCh
			continue
		}
		next := db.scheduledQueue[0]
		db.mu.Unlock()

		if wait := time.Until(next.publishAt); wait > 0 {
			time.Sleep(wait)
		}

		db.mu.Lock()
		// 只有本 goroutine 会出队，队首仍是 next。清零后再切片，避免底层数组继续引用消息。
		db.scheduledQueue[0] = scheduledMessage{}
		db.scheduledQueue = db.scheduledQueue[1:]
		db.mu.Unlock()

		db.broker.Publish(next.message, next.topic)
	}
}
