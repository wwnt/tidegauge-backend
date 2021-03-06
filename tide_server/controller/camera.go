package controller

import (
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/robfig/cron/v3"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"tide/common"
	"tide/pkg/custype"
	"tide/tide_server/auth"
	"tide/tide_server/db"
	"tide/tide_server/global"
	"time"
)

func ListCameraStatusPermission(c *gin.Context) {
	var (
		username string
		role     = c.GetInt(contextKeyRole)
	)
	if role == auth.NormalUser {
		username = c.GetString(contextKeyUsername)
	} else if role >= auth.Admin {
		username = c.Query("username")
	} else {
		return
	}

	if role >= auth.Admin && username == "" {
		stations, err := db.GetStations()
		if err != nil {
			logger.Error(err.Error())
			return
		}
		var permission = make(map[uuid.UUID]json.RawMessage)
		for _, item := range stations {
			permission[item.Id] = item.Cameras
		}
		c.JSON(http.StatusOK, permission)
	} else {
		permission, err := authorization.GetCameraStatusPermissions(username)
		if err != nil {
			logger.Error(err.Error())
			return
		}
		c.JSON(http.StatusOK, permission)
	}
}

func EditCameraStatusPermission(c *gin.Context) {
	var permission struct {
		Username string                 `json:"username" binding:"required"`
		Scopes   map[uuid.UUID][]string `json:"scopes"`
	}
	if err := c.Bind(&permission); err != nil {
		return
	}
	dstUser, err := userManager.GetUser(permission.Username)
	if err != nil {
		logger.Error(err.Error())
		return
	}
	// can not edit admins permission
	if dstUser.Role >= auth.Admin {
		return
	}
	if err = authorization.EditCameraStatusPermission(permission.Username, permission.Scopes); err != nil {
		logger.Error(err.Error())
		return
	}
	_, _ = c.Writer.Write([]byte("ok"))
}
func CameraLiveSnapshot(c *gin.Context) {
	name := c.Query("name")
	if name == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	s := c.Query("station_id")
	if s == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	stationId, err := uuid.Parse(s)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	value, ok := connections.Load(stationId)
	if !ok {
		return
	}
	stationConn, err := value.(*yamux.Session).Open()
	if err != nil {
		_ = c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	defer func() { _ = stationConn.Close() }()

	if _, err := stationConn.Write([]byte{common.MsgCameraSnapShot}); err != nil {
		_ = c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	if err = json.NewEncoder(stationConn).Encode(name); err != nil {
		return
	}
	c.Header("Content-Type", "image/jpeg")
	_, _ = io.Copy(c.Writer, stationConn)
}

type imgInfo struct {
	Millisecond custype.TimeMillisecond `json:"millisecond"`
	Bytes       []byte                  `json:"img"`
}

func CameraLatestSnapShot(c *gin.Context) {
	var params struct {
		CameraName string `form:"name" binding:"required"`
		StationId  string `form:"station_id" binding:"required"`
		After      int64  `form:"after"`
	}
	if c.Bind(&params) != nil {
		return
	}
	stationId, err := uuid.Parse(params.StationId)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if c.GetInt(contextKeyRole) < auth.Admin {
		if !authorization.CheckCameraStatusPermission(c.GetString(contextKeyUsername), stationId, params.CameraName) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
	}
	up, err := db.IsUpstreamStation(stationId)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	if up {
		var latestCacheTs custype.TimeMillisecond
		// get the latest cache time
		cachedDirs, err := os.ReadDir(path.Join(global.Config.Tide.Camera.Storage, stationId.String(), params.CameraName))
		if err != nil {
			logger.Error(err.Error())
			if !errors.Is(err, fs.ErrNotExist) {
				return
			} else {
				if err = os.MkdirAll(path.Join(global.Config.Tide.Camera.Storage, stationId.String(), params.CameraName), 0755); err != nil {
					logger.Error(err.Error())
					return
				}
			}
		} else if len(cachedDirs) > 0 {
			name := cachedDirs[len(cachedDirs)-1].Name()
			if len(name) != 17 { //millisecond.jpg
				return
			}
			ts, err := strconv.ParseInt(name[:13], 10, 0)
			if err != nil {
				logger.Error(err.Error())
				return
			}
			latestCacheTs = custype.TimeMillisecond(ts)
		}
		//get upstreams
		ups, err := db.GetUpstreamsByStationId(stationId)
		if err != nil {
			logger.Error(err.Error())
			return
		}
		//get update from upstream
		for _, upstream := range ups {
			value, ok := connections.Load(upstream.Id)
			if !ok {
				continue
			}
			up := value.(*upstreamStorage)
			resp, err := up.httpClient.Get(up.config.LatestSnapshot + "?station_id=" + stationId.String() + "&name=" + params.CameraName + "&after=" + latestCacheTs.String())
			if err != nil {
				logger.Error(err.Error())
				continue
			}
			if resp.StatusCode != http.StatusOK {
				_ = resp.Body.Close()
				return
			}
			var imgsFromUp []imgInfo
			err = json.NewDecoder(resp.Body).Decode(&imgsFromUp)
			if err != nil {
				logger.Error(err.Error())
				return
			}
			_ = resp.Body.Close()
			for _, img := range imgsFromUp {
				err = os.WriteFile(path.Join(global.Config.Tide.Camera.Storage, stationId.String(), params.CameraName, img.Millisecond.String()+".jpg"), img.Bytes, 0755)
				if err != nil {
					logger.Error(err.Error())
					return
				}
			}
			i := len(cachedDirs) + len(imgsFromUp) - global.Config.Tide.Camera.LatestSnapshotCount // Can't be more than upstream
			for _, dir := range cachedDirs {
				if i > 0 {
					i--
					_ = os.Remove(path.Join(global.Config.Tide.Camera.Storage, stationId.String(), params.CameraName, dir.Name()))
				}
			}

			var imgsReturn []imgInfo
			for _, img := range imgsFromUp {
				if int64(img.Millisecond) > params.After && len(imgsReturn) <= global.Config.Tide.Camera.LatestSnapshotCount {
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
				ts, err := strconv.ParseInt(name[:13], 10, 0)
				if err != nil {
					logger.Error(err.Error())
					return
				}
				if ts > params.After && len(imgsReturn) < global.Config.Tide.Camera.LatestSnapshotCount {
					all, err := os.ReadFile(path.Join(global.Config.Tide.Camera.Storage, stationId.String(), params.CameraName, name))
					if err != nil {
						logger.Error(err.Error())
						return
					}
					imgsReturn = append(imgsReturn, imgInfo{Millisecond: custype.TimeMillisecond(ts), Bytes: all})
				} else {
					goto WriteResponse
				}
			}
		WriteResponse:
			c.JSON(http.StatusOK, imgsReturn)
			break
		}
	} else {
		imgs, err := readLocalStationImgs(stationId, params.CameraName)
		if err != nil {
			_ = c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		var start = 0
		if len(imgs) > global.Config.Tide.Camera.LatestSnapshotCount {
			start = len(imgs) - global.Config.Tide.Camera.LatestSnapshotCount
		}
		imgs = imgs[start:]
		var imgsReturn []imgInfo
		for i := len(imgs) - 1; i >= 0; i-- {
			file, err := os.Open(path.Join(global.Config.Tide.Camera.Storage, stationId.String(), params.CameraName, imgs[i]))
			if err != nil {
				logger.Error(err.Error())
				return
			}
			info, err := file.Stat()
			if err != nil {
				logger.Error(err.Error())
				return
			}
			if info.ModTime().UnixMilli() <= params.After {
				break
			}
			all, err := io.ReadAll(file)
			if err != nil {
				logger.Error(err.Error())
				return
			}
			imgsReturn = append(imgsReturn, imgInfo{Millisecond: custype.ToTimeMillisecond(info.ModTime()), Bytes: all})
		}
		c.JSON(http.StatusOK, imgsReturn)
	}
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
		} else {
			if path.Ext(d.Name()) == ".jpg" {
				imgs = append(imgs, p)
			}
		}
		return nil
	})
	return imgs, err
}

func cameraStorage() {
	cr := cron.New(cron.WithParser(cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)))
	cr.Start()
	cameraStorage := global.Config.Tide.Camera.Storage
	if cameraStorage != "" {
		deleteFtp := func() {
			var monthAgo = time.Now().Add(-24 * 30 * time.Hour)
			dirs, err := os.ReadDir(cameraStorage) //stationIds
			if err != nil {
				logger.Error(err.Error())
				return
			}
			for _, dir := range dirs {
				dirs1, err := os.ReadDir(path.Join(cameraStorage, dir.Name())) //cameraNames
				if err != nil {
					logger.Error(err.Error())
					return
				}
				for _, dir1 := range dirs1 {
					dirs2, err := os.ReadDir(path.Join(cameraStorage, dir.Name(), dir1.Name())) //cameraNames
					if err != nil {
						logger.Error(err.Error())
						return
					}
					for _, dir2 := range dirs2 {
						if dir2.IsDir() {
							t, err := time.Parse("2006-01-02", dir2.Name())
							if err != nil {
								logger.Error(err.Error())
								continue
							}
							if t.Before(monthAgo) {
								if err = os.RemoveAll(path.Join(cameraStorage, dir.Name(), dir1.Name(), dir2.Name())); err != nil {
									logger.Error(err.Error())
								}
							}
						} else if path.Ext(dir2.Name()) == ".jpg" {
							ts, err := strconv.ParseInt(strings.TrimSuffix(dir2.Name(), ".jpg"), 10, 0)
							if err != nil {
								logger.Error(err.Error())
								continue
							}
							t := time.UnixMilli(ts)
							if t.Before(monthAgo) {
								if err = os.RemoveAll(path.Join(cameraStorage, dir.Name(), dir1.Name(), dir2.Name())); err != nil {
									logger.Error(err.Error())
								}
							}
						}
					}
				}
			}
		}
		deleteFtp()
		if _, err := cr.AddFunc("@daily", deleteFtp); err != nil {
			logger.Fatal(err.Error())
		}
	}
}
