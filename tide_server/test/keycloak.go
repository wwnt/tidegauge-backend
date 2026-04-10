package test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"tide/tide_server/global"

	"github.com/Nerzal/gocloak/v13"
)

const (
	AdminUsername = "tgm-admin"
	AdminPassword = "123456" // Just for test
)

func InitKeycloak(adminUsername string, adminPassword string, replaceRealm bool) {
	ctx := context.Background()
	kc := gocloak.NewClient(global.Config.Keycloak.BasePath, gocloak.SetAuthRealms("realms"), gocloak.SetAuthAdminRealms("admin/realms"))
	token, err := kc.LoginAdmin(ctx, global.Config.Keycloak.MasterUsername, global.Config.Keycloak.MasterPassword, "master")
	if err != nil {
		log.Fatal(err)
	}
createRealm:
	// Create Keycloak Security Realm for this application
	_, err = kc.CreateRealm(ctx, token.AccessToken, gocloak.RealmRepresentation{Realm: new(global.Config.Keycloak.Realm), Enabled: new(true)})
	if err != nil {
		if apiErr := err.(*gocloak.APIError); apiErr.Code != http.StatusConflict {
			log.Fatal(err)
		}
		if !replaceRealm {
			log.Fatal("realm " + global.Config.Keycloak.Realm + " already exists")
		}
		err = kc.DeleteRealm(ctx, token.AccessToken, global.Config.Keycloak.Realm)
		if err != nil {
			log.Fatal(err)
		}
		goto createRealm
	}

	var credentials = []gocloak.CredentialRepresentation{
		{
			Type:  new("password"),
			Value: new(adminPassword),
		},
	}

	// Create Superuser (Only the very first user)
	_, err = kc.CreateUser(ctx, token.AccessToken, global.Config.Keycloak.Realm,
		gocloak.User{Username: new(adminUsername), Enabled: new(true), Credentials: &credentials})
	if err != nil {
		log.Fatal(err)
	}

	// Create OAuth Client
	clientId, err := kc.CreateClient(ctx, token.AccessToken, global.Config.Keycloak.Realm, gocloak.Client{ClientID: new(global.Config.Keycloak.ClientId), DirectAccessGrantsEnabled: new(true)})
	if err != nil {
		log.Fatal(err)
	}

	// Get the Client Secret (Token)
	credential, err := kc.GetClientSecret(ctx, token.AccessToken, global.Config.Keycloak.Realm, clientId)
	if err != nil {
		log.Fatal(err)
	}
	global.Config.Keycloak.ClientSecret = *credential.Value
	fmt.Println("Client secret:", *credential.Value)
}
