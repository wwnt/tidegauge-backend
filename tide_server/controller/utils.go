package controller

import (
	"fmt"
	"net/http"
	"tide/common"
	"tide/pkg/pubsub"
)

func addAuthorization(req *http.Request, token string) {
	req.Header.Set("Authorization", fmt.Sprintf("%s %s", "Bearer", token))
}

func addJsonContentHeader(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
}

func addPostFormContentHeader(req *http.Request) {
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
}

func uuidStringsMapToTopics(permissions common.UUIDStringsMap) pubsub.TopicSet {
	var permissionTopics = make(pubsub.TopicSet)
	for sid, items := range permissions {
		for _, item := range items {
			permissionTopics[common.StationItemStruct{StationId: sid, ItemName: item}] = struct{}{}
		}
	}
	return permissionTopics
}
