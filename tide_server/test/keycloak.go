package test

import (
	"context"
	"github.com/Nerzal/gocloak/v11"
	"log"
	"net/http"
	"tide/tide_server/global"
)

const (
	TgmAdmin = "tgm-admin"
	Password = "123456"
)

func InitKeycloak() {
	ctx := context.Background()
	kc := gocloak.NewClient(global.Config.Keycloak.BasePath)
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
		err = kc.DeleteRealm(ctx, token.AccessToken, global.Config.Keycloak.Realm)
		if err != nil {
			log.Fatal(err)
		}
		goto createRealm
	}

	userId, err := kc.CreateUser(ctx, token.AccessToken, global.Config.Keycloak.Realm, gocloak.User{Username: gocloak.StringP(TgmAdmin), Enabled: gocloak.BoolP(true)})
	if err != nil {
		log.Fatal(err)
	}
	err = kc.SetPassword(ctx, token.AccessToken, userId, global.Config.Keycloak.Realm, Password, false)
	if err != nil {
		log.Fatal(err)
	}
	clientId, err := kc.CreateClient(ctx, token.AccessToken, global.Config.Keycloak.Realm, gocloak.Client{ClientID: gocloak.StringP(global.Config.Keycloak.ClientId), DirectAccessGrantsEnabled: gocloak.BoolP(true)})
	if err != nil {
		log.Fatal(err)
	}

	credential, err := kc.RegenerateClientSecret(ctx, token.AccessToken, global.Config.Keycloak.Realm, clientId)
	if err != nil {
		log.Fatal(err)
	}
	global.Config.Keycloak.ClientSecret = *credential.Value
}
