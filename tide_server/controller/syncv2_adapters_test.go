package controller

import (
	"context"
	"testing"

	"tide/pkg/pubsub"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncV2RelayFrameSource_StopDoesNotCloseSubscriberChannels(t *testing.T) {
	oldHub := hub
	dataBroker := pubsub.NewBroker()
	hub = NewSyncHub(
		dataBroker,
		pubsub.NewDelayedBroker(dataBroker, 0),
		pubsub.NewBroker(),
		pubsub.NewBroker(),
		pubsub.NewBroker(),
		nil,
		nil,
	)
	defer func() { hub = oldHub }()

	_, _, stop := (syncV2RelayFrameSource{}).Subscribe(context.Background(), "u1", nil)

	hub.subscribersMu.Lock()
	var configSubscriber *pubsub.Subscriber
	var dataSubscriber *pubsub.Subscriber
	for subscriber, connType := range hub.subscribersByUser["u1"] {
		if connType == connTypeSyncConfig {
			configSubscriber = subscriber
		}
		if connType == connTypeSyncData {
			dataSubscriber = subscriber
		}
	}
	hub.subscribersMu.Unlock()

	require.NotNil(t, configSubscriber)
	require.NotNil(t, dataSubscriber)

	stop()

	assert.NotPanics(t, func() {
		select {
		case configSubscriber.Ch <- SendMsgStruct{Type: kMsgUpdateAvailable}:
		default:
		}
		select {
		case dataSubscriber.Ch <- forwardDataStruct{Type: kMsgData}:
		default:
		}
	})
}
