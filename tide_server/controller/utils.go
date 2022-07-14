package controller

import (
	"fmt"
	"net/http"
	"tide/common"
	"tide/pkg/pubsub"
)

func addAuthorization(req *http.Request, token string) {
	req.Header.Set("Authorization", fmt.Sprintf("%s %s", "bearer", token))
}

func addJsonContentHeader(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
}

func addPostFormContentHeader(req *http.Request) {
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
}

func uuidStringsMapToTopic(permissions common.UUIDStringsMap) pubsub.TopicMap {
	var topic = make(pubsub.TopicMap)
	for sid, items := range permissions {
		for _, item := range items {
			topic[common.StationItemStruct{StationId: sid, ItemName: item}] = struct{}{}
		}
	}
	return topic
}
