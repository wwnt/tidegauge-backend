package controller

import (
	"context"
	"testing"
	"time"

	"tide/pkg/pubsub"
)

func TestSyncHub_NewSubscriber_NilMessageDoesNotStopWriterLoop(t *testing.T) {
	h := &SyncHub{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	called := make(chan struct{}, 2)
	subscriber := h.NewSubscriber(ctx, cancel, func(any) error {
		called <- struct{}{}
		return nil
	})

	subscriber.Ch <- nil
	subscriber.Ch <- "should-be-written"

	for i := 0; i < 2; i++ {
		select {
		case <-called:
		case <-time.After(time.Second):
			t.Fatal("expected nil not to stop writer loop")
		}
	}
}

func TestSyncHub_BrokerOverflowCancelsTrackedSubscriber(t *testing.T) {
	dataBroker := pubsub.NewBroker()
	h := NewSyncHub(
		dataBroker,
		pubsub.NewDelayedBroker(dataBroker, 0),
		pubsub.NewBroker(),
		pubsub.NewBroker(),
		pubsub.NewBroker(),
		nil,
		nil,
	)

	ctx, cancel := context.WithCancel(context.Background())
	subscriber := pubsub.NewSubscriber(1, cancel)
	subscriber.Ch <- "occupied"

	h.TrackSubscriber("u1", subscriber, connTypeSyncData)
	h.Subscribe(BrokerData, subscriber, nil)

	h.Publish(BrokerData, "new-message", nil)

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("expected dropped subscriber to cancel tracked subscriber")
	}
}

func TestSyncHub_UpdatePermissions_ConfigChannelFullCancelsSubscriber(t *testing.T) {
	truncateDB(t)

	dataBroker := pubsub.NewBroker()
	h := NewSyncHub(
		dataBroker,
		pubsub.NewDelayedBroker(dataBroker, 0),
		pubsub.NewBroker(),
		pubsub.NewBroker(),
		pubsub.NewBroker(),
		nil,
		nil,
	)

	ctx, cancel := context.WithCancel(context.Background())
	subscriber := pubsub.NewSubscriber(1, cancel)
	subscriber.Ch <- "occupied"

	h.TrackSubscriber("u1", subscriber, connTypeSyncConfig)
	h.UpdatePermissions("u1", nil)

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("expected full config channel to cancel subscriber")
	}
}
