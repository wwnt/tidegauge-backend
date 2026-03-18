package controller

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strconv"
	"time"

	"tide/common"
	"tide/pkg/custype"
	"tide/tide_server/auth"
	"tide/tide_server/db"
	"tide/tide_server/global"
	syncv2station "tide/tide_server/syncv2/station"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
)

func ListCameraStatusPermission(w http.ResponseWriter, r *http.Request) {
	var username string
	role := requestRole(r)
	if role == auth.NormalUser {
		username = requestUsername(r)
	} else if role >= auth.Admin {
		username = r.URL.Query().Get("username")
	} else {
		return
	}

	if role >= auth.Admin && username == "" {
		stations, err := db.GetStations()
		if err != nil {
			slog.Error("Failed to get stations for camera permissions", "error", err)
			return
		}
		permission := make(map[uuid.UUID]json.RawMessage)
		for _, item := range stations {
			permission[item.Id] = item.Cameras
		}
		writeJSON(w, http.StatusOK, permission)
		return
	}

	permission, err := authorization.GetCameraStatusPermissions(username)
	if err != nil {
		slog.Error("Failed to get camera status permissions", "username", username, "error", err)
		return
	}
	writeJSON(w, http.StatusOK, permission)
}

func EditCameraStatusPermission(w http.ResponseWriter, r *http.Request) {
	var permission struct {
		Username string                 `json:"username"`
		Scopes   map[uuid.UUID][]string `json:"scopes"`
	}
	if !readJSONOrBadRequest(w, r, &permission) {
		return
	}
	if permission.Username == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	dstUser, err := userManager.GetUser(permission.Username)
	if err != nil {
		slog.Error("Failed to get user for camera permission edit", "username", permission.Username, "error", err)
		return
	}
	// can not edit admins permission
	if dstUser.Role >= auth.Admin {
		return
	}
	if err = authorization.EditCameraStatusPermission(permission.Username, permission.Scopes); err != nil {
		slog.Error("Failed to edit camera status permission", "username", permission.Username, "error", err)
		return
	}
	writeOK(w)
}

func CameraLiveSnapshot(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	rawStationID := r.URL.Query().Get("station_id")
	if rawStationID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	stationID, err := uuid.Parse(rawStationID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if b, snapErr := v2SnapshotOrErr(stationID, name, 30*time.Second); snapErr == nil {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(b)
		return
	} else if !errors.Is(snapErr, syncv2station.ErrStationNotConnected) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	value, ok := recvConnections.Load(stationID)
	if !ok {
		return
	}
	stationConn, err := value.(*yamux.Session).Open()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer func() { _ = stationConn.Close() }()

	if _, err := stationConn.Write([]byte{common.MsgCameraSnapShot}); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err = json.NewEncoder(stationConn).Encode(name); err != nil {
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = io.Copy(w, stationConn)
}

type imgInfo struct {
	Millisecond custype.UnixMs `json:"millisecond"`
	Bytes       []byte         `json:"img"`
}

func CameraLatestSnapShot(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	cameraName := query.Get("name")
	stationIDRaw := query.Get("station_id")
	afterRaw := query.Get("after")
	if cameraName == "" || stationIDRaw == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	stationID, err := uuid.Parse(stationIDRaw)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	after, _ := strconv.ParseInt(afterRaw, 10, 64)

	if requestRole(r) < auth.Admin {
		if !authorization.CheckCameraStatusPermission(requestUsername(r), stationID, cameraName) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	isUp, err := db.IsUpstreamStation(stationID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// the station is from upstream
	if isUp {
		// get the latest cache time
		var latestCacheTS custype.UnixMs
		cachedDirs, err := os.ReadDir(path.Join(global.Config.Tide.Camera.Storage, stationID.String(), cameraName))
		if err != nil {
			slog.Error("Failed to read cached dirs", "error", err)
			if !errors.Is(err, fs.ErrNotExist) {
				return
			}
			if err = os.MkdirAll(path.Join(global.Config.Tide.Camera.Storage, stationID.String(), cameraName), 0755); err != nil {
				slog.Error("Failed to create cached dirs", "error", err)
				return
			}
		} else if len(cachedDirs) > 0 {
			name := cachedDirs[len(cachedDirs)-1].Name()
			if len(name) != 17 {
				return
			}
			ts, err := strconv.ParseInt(name[:13], 10, 64)
			if err != nil {
				slog.Error("Failed to parse timestamp", "error", err)
				return
			}
			latestCacheTS = custype.UnixMs(ts)
		}

		//get upstreams
		ups, err := db.GetUpstreamsByStationId(stationID)
		if err != nil {
			slog.Error("Failed to get upstreams", "error", err)
			return
		}
		for _, upstream := range ups {
			value, ok := recvConnections.Load(upstream.Id)
			if !ok {
				continue
			}
			upstreamState := value.(*upstreamSyncState)
			if upstreamState.httpClient == nil {
				slog.Error("Upstream auth client unavailable", "upstream_id", upstream.Id, "url", upstreamState.config.Url)
				continue
			}
			snapshotURL := upstreamState.config.Url + cameraLatestSnapshotPath + "?station_id=" + stationID.String() + "&name=" + cameraName + "&after=" + latestCacheTS.String()
			resp, err := upstreamState.httpClient.DoWithAuth(r.Context(), func(token string) (*http.Request, error) {
				req, reqErr := http.NewRequestWithContext(r.Context(), http.MethodGet, snapshotURL, nil)
				if reqErr != nil {
					return nil, reqErr
				}
				addAuthorization(req, token)
				return req, nil
			})
			if err != nil {
				slog.Error("Failed to get upstream snapshot", "error", err)
				continue
			}
			if resp.StatusCode != http.StatusOK {
				_ = resp.Body.Close()
				return
			}
			var imgsFromUp []imgInfo
			err = json.NewDecoder(resp.Body).Decode(&imgsFromUp)
			_ = resp.Body.Close()
			if err != nil {
				slog.Error("Failed to decode upstream snapshot", "error", err)
				return
			}
			for _, img := range imgsFromUp {
				err = os.WriteFile(path.Join(global.Config.Tide.Camera.Storage, stationID.String(), cameraName, img.Millisecond.String()+".jpg"), img.Bytes, 0755)
				if err != nil {
					slog.Error("Failed to write upstream snapshot", "error", err)
					return
				}
			}
			i := len(cachedDirs) + len(imgsFromUp) - global.Config.Tide.Camera.LatestSnapshotCount
			for _, dir := range cachedDirs {
				if i > 0 {
					i--
					_ = os.Remove(path.Join(global.Config.Tide.Camera.Storage, stationID.String(), cameraName, dir.Name()))
				}
			}

			var imgsReturn []imgInfo
			for _, img := range imgsFromUp {
				if int64(img.Millisecond) > after && len(imgsReturn) <= global.Config.Tide.Camera.LatestSnapshotCount {
					imgsReturn = append(imgsReturn, img)
				} else {
					goto WriteResponse
				}
			}
			for i := len(cachedDirs) - 1; i >= 0; i-- {
				name := cachedDirs[i].Name()
				if len(name) != 17 {
					return
				}
				ts, err := strconv.ParseInt(name[:13], 10, 64)
				if err != nil {
					slog.Error("Failed to parse timestamp", "error", err)
					return
				}
				if ts > after && len(imgsReturn) < global.Config.Tide.Camera.LatestSnapshotCount {
					all, err := os.ReadFile(path.Join(global.Config.Tide.Camera.Storage, stationID.String(), cameraName, name))
					if err != nil {
						slog.Error("Failed to read cached snapshot", "error", err)
						return
					}
					imgsReturn = append(imgsReturn, imgInfo{Millisecond: custype.UnixMs(ts), Bytes: all})
				} else {
					goto WriteResponse
				}
			}
		WriteResponse:
			writeJSON(w, http.StatusOK, imgsReturn)
			break
		}
		return
	}

	// the station is from local
	imgs, err := readLocalStationImgs(stationID, cameraName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	start := 0
	if len(imgs) > global.Config.Tide.Camera.LatestSnapshotCount {
		start = len(imgs) - global.Config.Tide.Camera.LatestSnapshotCount
	}
	imgs = imgs[start:]

	var imgsReturn []imgInfo
	for i := len(imgs) - 1; i >= 0; i-- {
		file, err := os.Open(path.Join(global.Config.Tide.Camera.Storage, stationID.String(), cameraName, imgs[i]))
		if err != nil {
			slog.Error("Failed to open local snapshot", "error", err)
			return
		}
		info, err := file.Stat()
		_ = file.Close()
		if err != nil {
			slog.Error("Failed to stat local snapshot", "error", err)
			return
		}
		if info.ModTime().UnixMilli() <= after {
			break
		}
		all, err := os.ReadFile(path.Join(global.Config.Tide.Camera.Storage, stationID.String(), cameraName, imgs[i]))
		if err != nil {
			slog.Error("Failed to read local snapshot", "error", err)
			return
		}
		imgsReturn = append(imgsReturn, imgInfo{Millisecond: custype.ToUnixMs(info.ModTime()), Bytes: all})
	}
	writeJSON(w, http.StatusOK, imgsReturn)
}

func readLocalStationImgs(stationId uuid.UUID, cameraName string) ([]string, error) {
	var imgs []string

	err := fs.WalkDir(os.DirFS(path.Join(global.Config.Tide.Camera.Storage, stationId.String(), cameraName)), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "dav" {
				return fs.SkipDir
			}
		} else if path.Ext(d.Name()) == ".jpg" {
			imgs = append(imgs, p)
		}
		return nil
	})
	return imgs, err
}
