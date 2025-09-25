package controller

import (
	"bytes"
	"encoding/json"
	"github.com/Nerzal/gocloak/v13"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"tide/common"
	"tide/tide_server/auth"
	"tide/tide_server/db"
	"tide/tide_server/test"
)

func TestWebapi(t *testing.T) {
	truncateDB(t)

	var items []db.Item
	t.Run("applyAccount", func(t *testing.T) {
		b, _ := json.Marshal(user01.UserBaseInfo)
		req, _ := http.NewRequest(http.MethodPost, "/applyAccount", bytes.NewReader(b))
		addJsonContentHeader(req)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		assert.Equal(t, "ok", w.Body.String())
	})

	t.Run("login_superAdmin", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, loginPath, strings.NewReader("username="+test.AdminUsername+"&password="+test.AdminPassword))
		addPostFormContentHeader(req)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, 200, w.Code)

		var jwt gocloak.JWT
		err := json.Unmarshal(w.Body.Bytes(), &jwt)
		require.NoError(t, err)

		token := jwt.AccessToken

		t.Run("createStation", func(t *testing.T) {
			b, _ := json.Marshal(db.Station{Identifier: station1.Identifier, Name: station1.Name})
			req, _ := http.NewRequest(http.MethodPost, "/editStation", bytes.NewReader(b))
			addJsonContentHeader(req)
			addAuthorization(req, token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code)
			assert.Equal(t, "ok", w.Body.String())
		})

		t.Run("listStation", func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, "/listStation", nil)
			addAuthorization(req, token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code)
			var data []db.Station
			err := json.Unmarshal(w.Body.Bytes(), &data)
			assert.NoError(t, err)
			assert.Equal(t, 1, len(data))
			station1 = data[0]
		})

		t.Run("delStation", func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, "/delStation", strings.NewReader(`id=`+station1.Id.String()))
			addPostFormContentHeader(req)
			addAuthorization(req, token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, 200, w.Code)
			require.Equal(t, "ok", w.Body.String())
		})

		t.Run("recreateStation", func(t *testing.T) {
			tmpStation1 := station1
			tmpStation1.Id = uuid.Nil // to create
			b, _ := json.Marshal(tmpStation1)
			req, _ := http.NewRequest(http.MethodPost, "/editStation", bytes.NewReader(b))
			addJsonContentHeader(req)
			addAuthorization(req, token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code)
			assert.Equal(t, "ok", w.Body.String())
		})

		t.Run("station_conn", func(t *testing.T) {
			conn1, conn2 := net.Pipe() // conn1: station client, conn2:station server
			go func() {
				mockStationClient(t, conn1, station1Info)
				_ = conn1.Close()
			}()
			defer func() { _ = conn2.Close() }()
			handleStationConnStream1(conn2, nil)
		})

		t.Run("listDevice", func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, "/listDevice", nil)
			addAuthorization(req, token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code)
			var data []db.Device
			err := json.Unmarshal(w.Body.Bytes(), &data)
			assert.NoError(t, err)
			assert.Equal(t, len(station1Info.Devices), len(data))
		})

		t.Run("listItem", func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, "/listItem?station_id="+station1.Id.String(), nil)
			addAuthorization(req, token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code)

			err := json.Unmarshal(w.Body.Bytes(), &items)
			require.NoError(t, err)
			assert.Equal(t, 2, len(items))
		})

		record := db.DeviceRecord{
			Id:         uuid.Nil,
			StationId:  station1.Id,
			DeviceName: "device1",
			Record:     "some describe",
		}
		t.Run("editDeviceRecord", func(t *testing.T) {
			b, _ := json.Marshal(record)
			req, _ := http.NewRequest(http.MethodPost, "/editDeviceRecord", bytes.NewReader(b))
			addJsonContentHeader(req)
			addAuthorization(req, token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code)
			assert.Equal(t, "ok", w.Body.String())
		})

		t.Run("listDeviceRecord", func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, "/listDeviceRecord", nil)
			addAuthorization(req, token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code)
			var data []db.DeviceRecord
			err := json.Unmarshal(w.Body.Bytes(), &data)
			require.NoError(t, err)
			assert.Equal(t, 1, len(data))
		})

		t.Run("passApplication", func(t *testing.T) {
			b, _ := json.Marshal([]string{user01.Username})
			req, _ := http.NewRequest(http.MethodPost, "/passApplication", bytes.NewReader(b))
			addAuthorization(req, token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code)
			require.Equal(t, "ok", w.Body.String())
		})

		t.Run("listUser", func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, "/listUser?application=true", nil)
			addAuthorization(req, token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code)
			var data []auth.User
			err := json.Unmarshal(w.Body.Bytes(), &data)
			require.NoError(t, err)
			require.Equal(t, 1, len(data))
		})

		t.Run("editUser_by_superAdmin", func(t *testing.T) {
			user01.LiveCamera = true
			b, _ := json.Marshal(user01)
			req, _ := http.NewRequest(http.MethodPost, "/editUser", bytes.NewReader(b))
			addJsonContentHeader(req)
			addAuthorization(req, token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code)
			assert.Equal(t, "ok", w.Body.String())
		})

		t.Run("editPermission", func(t *testing.T) {
			b, _ := json.Marshal(editPermissionStruct{
				Username: user01.Username, Permissions: map[uuid.UUID][]string{station1.Id: {items[0].Name}}})
			req, _ := http.NewRequest(http.MethodPost, "/editPermission", bytes.NewReader(b))
			addJsonContentHeader(req)
			addAuthorization(req, token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code)
			assert.Equal(t, "ok", w.Body.String())
		})

	})

	t.Run("login_user01", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, loginPath, strings.NewReader("username="+user01.Username+"&password="+user01.Password))
		addPostFormContentHeader(req)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, 200, w.Code)

		var jwt gocloak.JWT
		err := json.Unmarshal(w.Body.Bytes(), &jwt)
		require.NoError(t, err)

		token := jwt.AccessToken

		t.Run("editUser_by_user01", func(t *testing.T) {
			user01.Email = "user01@example.com"
			b, _ := json.Marshal(user01)
			req, _ := http.NewRequest(http.MethodPost, "/editUser", bytes.NewReader(b))
			addJsonContentHeader(req)
			addAuthorization(req, token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code)
			assert.Equal(t, "ok", w.Body.String())
		})

		t.Run("listPermission", func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, "/listPermission", nil)
			addAuthorization(req, token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code)
			var data common.UUIDStringsMap
			err := json.Unmarshal(w.Body.Bytes(), &data)
			require.NoError(t, err)
			require.Equal(t, 1, len(data))
		})

		t.Run("websocket", func(t *testing.T) {
			t.Skip()
			t.Run("data", func(t *testing.T) {
				var header = make(http.Header)
				header.Set("Cookie", "token="+token)
				url := "ws" + strings.TrimPrefix(testServer.URL, "http")
				t.Log(url)
				ws, _, err := websocket.DefaultDialer.Dial(url+"/ws/data", header)
				require.NoError(t, err)

				b, _ := json.Marshal(permissions)
				err = ws.WriteMessage(websocket.TextMessage, b)
				require.NoError(t, err)

				tmpData := forwardDataStruct{
					StationItemStruct: common.StationItemStruct{StationId: station1.Id, ItemName: items[0].Name},
					DataTimeStruct:    common.DataTimeStruct{Value: 0, Millisecond: 0},
				}
				go func() {
					log.Println(tmpData)
					err = dataPubSub.Publish(tmpData, common.StationItemStruct{
						StationId: station1.Id,
						ItemName:  items[0].Name,
					})
					require.NoError(t, err)
				}()

				var data forwardDataStruct
				err = ws.ReadJSON(&data)
				require.NoError(t, err)

				assert.Equal(t, tmpData, data)
				{

				}
			})
		})
	})
}
