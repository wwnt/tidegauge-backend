package pubsub

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPubSub_Unsubscribe(t *testing.T) {
	p := NewBroker()
	subscriber := NewSubscriber(1000, nil)
	p.Subscribe(subscriber, TopicSet{"123": struct{}{}})
	p.Unsubscribe(subscriber)
	_, ok := p.subscriptions.Load(subscriber)
	assert.False(t, ok)
}

func TestPubSub_RestrictTopics(t *testing.T) {
	subscriber := NewSubscriber(1000, nil)
	p := NewBroker()
	p.Subscribe(subscriber, TopicSet{"1": struct{}{}, "2": struct{}{}})
	p.RestrictTopics(subscriber, TopicSet{"3": struct{}{}, "2": struct{}{}})

	v, ok := p.subscriptions.Load(subscriber)
	assert.True(t, ok)
	val, _ := v.(TopicSet)
	assert.Equal(t, TopicSet{"2": struct{}{}}, val)
}

func TestPubSub_Publish(t *testing.T) {
	t.Run("direct channel receives any values", func(t *testing.T) {
		type testStruct struct {
			Name string
			Val  int
		}
		p := NewBroker()
		subscriber := NewSubscriber(10, nil)
		p.Subscribe(subscriber, nil)

		sent := testStruct{Name: "hello", Val: 42}
		p.Publish(sent, nil)

		got := <-subscriber.Ch
		assert.Equal(t, sent, got)
	})

	t.Run("topic filtering", func(t *testing.T) {
		p := NewBroker()
		subscriber := NewSubscriber(10, nil)
		p.Subscribe(subscriber, TopicSet{"a": struct{}{}})

		p.Publish("match", "a")
		p.Publish("no-match", "b")

		assert.Equal(t, "match", <-subscriber.Ch)
		assert.Equal(t, 0, len(subscriber.Ch))
	})
}

func TestPubSub_Subscribe(t *testing.T) {
	p := NewBroker()
	subscriber := NewSubscriber(1000, nil)
	p.Subscribe(subscriber, TopicSet{"1": struct{}{}})
	_, ok := p.subscriptions.Load(subscriber)
	assert.True(t, ok)
}

func TestDelayedBroker_DeliversInOrderAfterDelay(t *testing.T) {
	const delay = 30 * time.Millisecond
	broker := NewBroker()
	subscriber := NewSubscriber(10, nil)
	broker.Subscribe(subscriber, nil)
	delayed := NewDelayedBroker(broker, delay)

	start := time.Now()
	for i := 0; i < 3; i++ {
		delayed.Publish(i, nil)
	}
	for i := 0; i < 3; i++ {
		select {
		case got := <-subscriber.Ch:
			assert.Equal(t, i, got)
		case <-time.After(2 * time.Second):
			t.Fatalf("message %d was not delivered", i)
		}
	}
	assert.GreaterOrEqual(t, time.Since(start), delay)
}

func TestDelayedBroker_ZeroDelayPublishesImmediately(t *testing.T) {
	broker := NewBroker()
	subscriber := NewSubscriber(1, nil)
	broker.Subscribe(subscriber, nil)

	NewDelayedBroker(broker, 0).Publish("now", nil)

	assert.Equal(t, "now", <-subscriber.Ch)
}

// 发布间隔约等于延迟时，队列会反复被取空；旧实现在"取空后、重新等待前"收到新消息会
// 与发布方互相等待而永久卡死。
func TestDelayedBroker_PublishWhileQueueDrainsDoesNotDeadlock(t *testing.T) {
	const (
		delay = time.Millisecond
		total = 2000
	)
	broker := NewBroker()
	subscriber := NewSubscriber(total, nil)
	broker.Subscribe(subscriber, nil)
	delayed := NewDelayedBroker(broker, delay)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < total; i++ {
			delayed.Publish(i, nil)
			if i%2 == 0 {
				time.Sleep(delay)
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Publish blocked: delayed broker deadlocked")
	}
	for i := 0; i < total; i++ {
		select {
		case got := <-subscriber.Ch:
			if got != i {
				t.Fatalf("message %d delivered out of order: got %v", i, got)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("message %d was never delivered", i)
		}
	}
}

func TestPubSub_PublishOverflowTriggersHandler(t *testing.T) {
	p := NewBroker()
	called := false
	subscriber := NewSubscriber(1, func() {
		called = true
	})
	p.Subscribe(subscriber, nil)
	subscriber.Ch <- "occupied"

	p.Publish("next", nil)

	_, ok := p.subscriptions.Load(subscriber)
	assert.False(t, ok)
	assert.True(t, called)
}
