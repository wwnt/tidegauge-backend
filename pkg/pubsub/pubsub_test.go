package pubsub

import (
	"testing"

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
