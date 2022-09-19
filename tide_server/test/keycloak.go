package test

import (
	"context"
	"fmt"
	"github.com/Nerzal/gocloak/v11"
	"log"
	"net/http"
	"tide/tide_server/global"
)

const (
	AdminUsername = "tgm-admin"
	AdminPassword = "123456"
)

func InitKeycloak(adminUsername string, adminPassword string, replaceRealm bool) {
	ctx := context.Background()
	kc := gocloak.NewClient(global.Config.Keycloak.BasePath, gocloak.SetAuthRealms("realms"), gocloak.SetAuthAdminRealms("admin/realms"))
	token, err := kc.LoginAdmin(ctx, global.Config.Keycloak.MasterUsername, global.Config.Keycloak.MasterPassword, "master")
	if err != nil {
		log.Fatal(err)
	}
createRealm:
	_, err = kc.CreateRealm(ctx, token.AccessToken, gocloak.RealmRepresentation{Realm: gocloak.StringP(global.Config.Keycloak.Realm), Enabled: gocloak.BoolP(true)})
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
			Type:  gocloak.StringP("password"),
			Value: gocloak.StringP(adminPassword),
		},
	}

	_, err = kc.CreateUser(ctx, token.AccessToken, global.Config.Keycloak.Realm,
		gocloak.User{Username: gocloak.StringP(adminUsername), Enabled: gocloak.BoolP(true), Credentials: &credentials})
	if err != nil {
		log.Fatal(err)
	}

	clientId, err := kc.CreateClient(ctx, token.AccessToken, global.Config.Keycloak.Realm, gocloak.Client{ClientID: gocloak.StringP(global.Config.Keycloak.ClientId), DirectAccessGrantsEnabled: gocloak.BoolP(true)})
	if err != nil {
		log.Fatal(err)
	}

	credential, err := kc.GetClientSecret(ctx, token.AccessToken, global.Config.Keycloak.Realm, clientId)
	if err != nil {
		log.Fatal(err)
	}
	global.Config.Keycloak.ClientSecret = *credential.Value
	fmt.Println("client secret:", *credential.Value)
}
