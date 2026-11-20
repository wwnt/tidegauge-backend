package controller

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
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
	if role >= auth.Admin {
		username = r.URL.Query().Get("username")
	} else {
		username = requestUsername(r)
	}

	if role >= auth.Admin && username == "" {
		stations, err := db.GetStations()
		if err != nil {
			slog.Error("Failed to get stations for camera permissions", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
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
		w.WriteHeader(http.StatusInternalServerError)
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
		if errors.Is(err, auth.ErrUserNotFound) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		slog.Error("Failed to get user for camera permission edit", "username", permission.Username, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// can not edit admins permission
	if dstUser.Role >= auth.Admin {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if err = authorization.EditCameraStatusPermission(permission.Username, permission.Scopes); err != nil {
		slog.Error("Failed to edit camera status permission", "username", permission.Username, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeOK(w)
}

// validCameraName 拒绝会改变路径含义的摄像头名。名字来自查询参数，会被拼进快照缓存
// 目录的路径和发往上游的 URL。
func validCameraName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`) && !strings.ContainsRune(name, 0)
}

func CameraLiveSnapshot(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if !validCameraName(name) {
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

	// 注意：这里没有按摄像头查权限表，只有账号级的 live_camera 开关
	// （validateLiveSnapshotMiddleware）。是否要像 /cameraLatestSnapshot 那样再按
	// permissions_camera_status 收紧，是权限模型上的取舍，现状保持不变。

	if b, snapErr := v2SnapshotOrErr(stationID, name, 30*time.Second); snapErr == nil {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(b)
		return
	} else if !errors.Is(snapErr, syncv2station.ErrStationNotConnected) {
		slog.Warn("Live snapshot via sync v2 failed", "station_id", stationID, "camera", name, "error", snapErr)
		w.WriteHeader(http.StatusBadGateway)
		return
	}

	value, ok := recvConnections.Load(stationID)
	if !ok {
		// 站点不在线。
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	stationConn, err := value.(*yamux.Session).Open()
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer func() { _ = stationConn.Close() }()

	if _, err := stationConn.Write([]byte{common.MsgCameraSnapShot}); err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	if err = json.NewEncoder(stationConn).Encode(name); err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	// v1 协议没有错误帧：站点抓图失败时直接关流，一个字节都不会发。先读完再决定状态码，
	// 不能把空响应当成一张图片返回 200。快照大小与 v2 的帧上限一致。
	img, err := io.ReadAll(io.LimitReader(stationConn, maxLiveSnapshotBytes))
	if err != nil || len(img) == 0 {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(img)
}

// maxLiveSnapshotBytes 是经 v1 连接转发的单张实时快照的上限。
const maxLiveSnapshotBytes = 32 << 20

type imgInfo struct {
	Millisecond custype.UnixMs `json:"millisecond"`
	Bytes       []byte         `json:"img"`
}

func CameraLatestSnapShot(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	cameraName := query.Get("name")
	stationIDRaw := query.Get("station_id")
	afterRaw := query.Get("after")
	if !validCameraName(cameraName) || stationIDRaw == "" {
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
		if errors.Is(err, sql.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	// the station is from upstream
	if isUp {
		// 上游分支里的失败路径原先都是直接 return（客户端收到 200 空响应），
		// 统一用这个标记兜底成 502。
		responded := false
		defer func() {
			if !responded {
				w.WriteHeader(http.StatusBadGateway)
			}
		}()

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
			snapshotURL := upstreamState.config.Url + cameraLatestSnapshotPath + "?station_id=" + stationID.String() + "&name=" + url.QueryEscape(cameraName) + "&after=" + latestCacheTS.String()
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
			responded = true
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
