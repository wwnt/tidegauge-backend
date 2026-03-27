package controller

import (
	"context"
	"log/slog"
	"sync"

	"tide/common"
	"tide/pkg/pubsub"
	"tide/tide_server/auth"
	"tide/tide_server/db"

	"github.com/google/uuid"
)

// SyncHub centralizes all pubsub instances and connection tracking.
type SyncHub struct {
	dataBroker        *pubsub.Broker
	delayedDataBroker *pubsub.DelayedBroker
	missingDataBroker *pubsub.Broker
	statusBroker      *pubsub.Broker
	configBroker      *pubsub.Broker

	userManager      auth.UserManager
	permissionLoader auth.Permission

	subscribersMu     sync.Mutex
	subscribersByUser map[string]map[*pubsub.Subscriber]int
}

// NewSyncHub creates a SyncHub with the given pubsub instances and auth deps.
func NewSyncHub(
	dataBroker *pubsub.Broker,
	delayedDataBroker *pubsub.DelayedBroker,
	missingDataBroker *pubsub.Broker,
	statusBroker *pubsub.Broker,
	configBroker *pubsub.Broker,
	userManager auth.UserManager,
	permissionLoader auth.Permission,
) *SyncHub {
	h := &SyncHub{
		dataBroker:        dataBroker,
		delayedDataBroker: delayedDataBroker,
		missingDataBroker: missingDataBroker,
		statusBroker:      statusBroker,
		configBroker:      configBroker,
		userManager:       userManager,
		permissionLoader:  permissionLoader,
		subscribersByUser: make(map[string]map[*pubsub.Subscriber]int),
	}
	return h
}

// TrackSubscriber registers a subscriber channel for the given user.
func (h *SyncHub) TrackSubscriber(username string, subscriber *pubsub.Subscriber, connType int) {
	h.subscribersMu.Lock()
	defer h.subscribersMu.Unlock()
	if _, ok := h.subscribersByUser[username]; !ok {
		h.subscribersByUser[username] = make(map[*pubsub.Subscriber]int)
	}
	h.subscribersByUser[username][subscriber] = connType
}

// UntrackSubscriber removes a subscriber channel for the given user.
func (h *SyncHub) UntrackSubscriber(username string, subscriber *pubsub.Subscriber) {
	h.subscribersMu.Lock()
	defer h.subscribersMu.Unlock()
	delete(h.subscribersByUser[username], subscriber)
	if len(h.subscribersByUser[username]) == 0 {
		delete(h.subscribersByUser, username)
	}
}

// DisconnectUser cancels all matching connections for a user.
func (h *SyncHub) DisconnectUser(username string, connTypeMask int) {
	h.subscribersMu.Lock()
	defer h.subscribersMu.Unlock()
	for subscriber, connType := range h.subscribersByUser[username] {
		if connTypeMask&connType != 0 {
			subscriber.Cancel()
		}
	}
}

// DisconnectAll cancels all matching connections for all users.
func (h *SyncHub) DisconnectAll(connTypeMask int) {
	h.subscribersMu.Lock()
	defer h.subscribersMu.Unlock()
	for _, userSubscribers := range h.subscribersByUser {
		for subscriber, connType := range userSubscribers {
			if connTypeMask&connType != 0 {
				subscriber.Cancel()
			}
		}
	}
}

// UpdatePermissions handles a permission change for a user.
// - WebBrowser connections: limit topic scope
// - SyncData connections: cancel (reconnect will pick up new permissions)
// - SyncConfig connections: send updated available items
func (h *SyncHub) UpdatePermissions(username string, userPermissions map[uuid.UUID][]string) {
	h.subscribersMu.Lock()
	defer h.subscribersMu.Unlock()

	var allowedTopics pubsub.TopicSet
	if userPermissions != nil {
		allowedTopics = uuidStringsMapToTopics(userPermissions)
	}
	var configSubscribers []*pubsub.Subscriber
	for subscriber, connType := range h.subscribersByUser[username] {
		if connTypeWebBrowser&connType != 0 {
			h.dataBroker.RestrictTopics(subscriber, allowedTopics)
		}
		if connTypeSyncData&connType != 0 {
			subscriber.Cancel()
		}
		if connTypeSyncConfig&connType != 0 {
			configSubscribers = append(configSubscribers, subscriber)
		}
	}
	if len(configSubscribers) > 0 {
		availableItems, err := db.GetAvailableItems()
		if err != nil {
			slog.Error("Failed to get available items for permission change", "username", username, "error", err)
			for _, subscriber := range configSubscribers {
				subscriber.Cancel()
			}
			return
		}
		filteredAvailableItems := make(common.UUIDStringsMap)
		for _, stationItem := range availableItems {
			if _, ok := allowedTopics[stationItem]; ok || allowedTopics == nil {
				filteredAvailableItems[stationItem.StationId] = append(filteredAvailableItems[stationItem.StationId], stationItem.ItemName)
			}
		}
		msg := SendMsgStruct{Type: kMsgUpdateAvailable, Body: filteredAvailableItems}
		for _, subscriber := range configSubscribers {
			select {
			case subscriber.Ch <- msg:
			default:
				subscriber.Cancel()
			}
		}
	}
}

// BroadcastAvailableChange sends updated available items to all SyncConfig connections,
// filtered by each user's permissions.
func (h *SyncHub) BroadcastAvailableChange(availableItemsByStation map[uuid.UUID][]string) {
	h.subscribersMu.Lock()
	defer h.subscribersMu.Unlock()
	for username, userSubscribers := range h.subscribersByUser {
		var configSubscribers []*pubsub.Subscriber
		for subscriber, connType := range userSubscribers {
			if connTypeSyncConfig&connType != 0 {
				configSubscribers = append(configSubscribers, subscriber)
			}
		}
		if len(configSubscribers) == 0 {
			continue
		}
		user, err := h.userManager.GetUser(username)
		if err != nil {
			slog.Error("Failed to get user for available change", "error", err)
			for _, subscriber := range configSubscribers {
				subscriber.Cancel()
			}
			continue
		}
		var allowedTopics pubsub.TopicSet
		if user.Role == auth.NormalUser {
			userPermissions, err := h.permissionLoader.GetPermissions(username)
			if err != nil {
				slog.Error("Failed to get permissions for available change", "error", err)
				for _, subscriber := range configSubscribers {
					subscriber.Cancel()
				}
				continue
			}
			allowedTopics = uuidStringsMapToTopics(userPermissions)
		}
		filteredAvailableItems := make(common.UUIDStringsMap)
		for stationID, itemNames := range availableItemsByStation {
			for _, itemName := range itemNames {
				if _, ok := allowedTopics[common.StationItemStruct{StationId: stationID, ItemName: itemName}]; ok || allowedTopics == nil {
					filteredAvailableItems[stationID] = append(filteredAvailableItems[stationID], itemName)
				}
			}
		}
		msg := SendMsgStruct{Type: kMsgUpdateAvailable, Body: filteredAvailableItems}
		for _, subscriber := range configSubscribers {
			select {
			case subscriber.Ch <- msg:
			default:
				subscriber.Cancel()
			}
		}
	}
}

// BroadcastAddItems queries available items from DB, then broadcasts to SyncConfig connections.
func (h *SyncHub) BroadcastAddItems() {
	availableItems, err := db.GetAvailableItems()
	if err != nil {
		slog.Error("Failed to get available items", "error", err)
		return
	}
	// Convert []common.StationItemStruct to map[uuid.UUID][]string
	availableItemsByStation := make(map[uuid.UUID][]string)
	for _, stationItem := range availableItems {
		availableItemsByStation[stationItem.StationId] = append(availableItemsByStation[stationItem.StationId], stationItem.ItemName)
	}
	h.BroadcastAvailableChange(availableItemsByStation)
}

// NewSubscriber creates a subscriber with a background drain goroutine.
// Use for callers that need the subscriber (e.g. dynamic topic changes in DataWebsocket).
// The caller is responsible for Subscribe/Unsubscribe lifecycle management.
func (h *SyncHub) NewSubscriber(ctx context.Context, cancel context.CancelFunc, writeMessage func(any) error) *pubsub.Subscriber {
	subscriber := pubsub.NewSubscriber(1000, cancel)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case message, ok := <-subscriber.Ch:
				if !ok {
					return
				}
				if writeMessage(message) != nil {
					subscriber.Cancel()
					return
				}
			}
		}
	}()
	return subscriber
}

// BrokerKind identifies a broker managed by SyncHub.
type BrokerKind int

const (
	BrokerData BrokerKind = iota
	BrokerMissingData
	BrokerStatus
	BrokerConfig
)

func (h *SyncHub) brokerByKind(kind BrokerKind) *pubsub.Broker {
	switch kind {
	case BrokerData:
		return h.dataBroker
	case BrokerMissingData:
		return h.missingDataBroker
	case BrokerStatus:
		return h.statusBroker
	case BrokerConfig:
		return h.configBroker
	default:
		return nil
	}
}

// Publish sends a message through the selected broker.
func (h *SyncHub) Publish(kind BrokerKind, message any, topic any) {
	broker := h.brokerByKind(kind)
	if broker == nil {
		return
	}
	broker.Publish(message, topic)
}

// PublishDelayedData publishes a delayed data message.
func (h *SyncHub) PublishDelayedData(message any, topic any) {
	h.delayedDataBroker.Publish(message, topic)
}

// Subscribe registers a subscriber on the selected broker.
func (h *SyncHub) Subscribe(kind BrokerKind, subscriber *pubsub.Subscriber, topics pubsub.TopicSet) {
	broker := h.brokerByKind(kind)
	if broker == nil {
		return
	}
	broker.Subscribe(subscriber, topics)
}

// Unsubscribe removes a subscriber from the selected broker.
func (h *SyncHub) Unsubscribe(kind BrokerKind, subscriber *pubsub.Subscriber) {
	broker := h.brokerByKind(kind)
	if broker == nil {
		return
	}
	broker.Unsubscribe(subscriber)
}
