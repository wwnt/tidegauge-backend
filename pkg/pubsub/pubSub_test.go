package pubsub

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"testing"
)

func TestPubSub_Evict(t *testing.T) {
	var (
		conn1, _ = net.Pipe()
		p        = NewPubSub()
		key      = "123"
	)
	subscriber := NewSubscriber(nil, conn1)
	p.SubscribeTopic(subscriber, TopicMap{key: struct{}{}})
	p.Evict(subscriber)
	_, ok := p.subscribers.Load(conn1)
	assert.False(t, ok)
}

func TestPubSub_EvictAndClose(t *testing.T) {
	var (
		conn1, _ = net.Pipe()
		p        = NewPubSub()
		key      = "123"
	)
	subscriber := NewSubscriber(nil, conn1)
	p.SubscribeTopic(subscriber, TopicMap{key: struct{}{}})
	p.EvictAndClose(subscriber)
	_, ok := p.subscribers.Load(conn1)
	assert.False(t, ok)
	_, err := conn1.Write([]byte{0})
	assert.ErrorIs(t, err, io.ErrClosedPipe)
}

func TestPubSub_LimitTopicScope(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "1",
			test: func(t *testing.T) {
				conn1, _ := net.Pipe()
				subscriber := NewSubscriber(nil, conn1)
				p := NewPubSub()
				p.SubscribeTopic(subscriber, TopicMap{"1": struct{}{}, "2": struct{}{}})
				p.LimitTopicScope(subscriber, TopicMap{"3": struct{}{}, "2": struct{}{}})
				val, ok := p.subscribers.Load(conn1)
				assert.True(t, ok)
				assert.Equal(t, TopicMap{"2": struct{}{}}, val)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.test(t)
		})
	}
}

func TestPubSub_Publish(t *testing.T) {
	type args struct {
		data   any
		subKey any
	}
	tests := []struct {
		name    string
		args    args
		test    func(t *testing.T)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "1",
			test: func(t *testing.T) {
				var key = "1"
				var data = "123"
				p := NewPubSub()
				conn1, conn2 := net.Pipe()
				subscriber := NewSubscriber(nil, conn1)
				ch := make(chan any, 1)
				go func() {
					var val any
					err := json.NewDecoder(conn2).Decode(&val)
					require.NoError(t, err)
					ch <- val
				}()
				p.SubscribeTopic(subscriber, TopicMap{key: struct{}{}})
				err := p.Publish(data, key)
				assert.NoError(t, err)
				assert.Equal(t, data, <-ch)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.test(t)
		})
	}
}

func TestPubSub_SubscribeTopic(t *testing.T) {
	var (
		conn1, _ = net.Pipe()
		p        = NewPubSub()
		key      = "1"
	)
	subscriber := NewSubscriber(nil, conn1)
	p.SubscribeTopic(subscriber, TopicMap{key: struct{}{}})
	_, ok := p.subscribers.Load(conn1)
	assert.True(t, ok)
}
