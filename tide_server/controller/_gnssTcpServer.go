package controller

import (
	"bufio"
	"log"
	"math"
	"net"
	"strconv"
	"strings"
	"tide/common"
	"tide/db"
	"time"
)

//var (
//	geo = ellipsoid.Init("WGS84", ellipsoid.Degrees, ellipsoid.Meter, true, true)
//)
type (
	gnssData struct {
		sid  string
		time time.Time
		xyzenu
	}
	xyzenu struct {
		xyz
		enu
	}
	xyz struct {
		X, Y, Z float64
	}
	enu struct {
		E, N, U float64
	}
)

func gnssTcpServer() {
	ln, err := net.Listen("tcp", global.Config.gnss.Listen)
	if err != nil {
		log.Fatal(err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer Recover()
			defer func() {
				_ = conn.Close()
			}()
			scanner := bufio.NewScanner(conn)
			if !scanner.Scan() {
				return
			}
			ss := strings.Split(scanner.Text(), ",")
			if len(ss) != 10 {
				return
			}
			var (
				biasedBuf        = make([]gnssData, 0, 5)
				tolerant         bool
				gnssData         gnssData
				dataHis          = db.DataHistory{StationId: 1, ItemName: ss[0]}
				alertHis         = db.AlertHistory{Typ: GNSSType}
				preE, preN, preU float64
				preTs            int64 //上一条数据时间戳
				exit             = make(chan struct{})
			)
			defer close(exit) // 退出 goroutine，并更新状态
			// 定时检查是否接收到数据
			go checkConnection(&dataHis, exit, conn)

			recordDeliver := func() {
				if thresholds[common.StationItemStruct{1, GNSSType}].condition != nil {
					alertHis.Sid = gnssData.sid
					alertHis.Time = gnssData.time
					alertHis.Timestamp = gnssData.time.Unix()
					alertHis.Diff = false

					alertHis.Item = "E"
					alertHis.Value = gnssData.E
					if handleAlert(&alertHis) {
						if err := db.SaveAlert(alert); err != nil {
							logger.Error(err.Error())
						}
						if err := alertPubSub.Publish(alert, nil); err != nil {
							logger.DPanic(err.Error())
						}
					}

					alertHis.Item = "N"
					alertHis.Value = gnssData.N
					handleAlert(&alertHis)

					alertHis.Item = "U"
					alertHis.Value = gnssData.U
					handleAlert(&alertHis)
					if preE != 0 || preN != 0 || preU != 0 {
						alertHis.Diff = true

						alertHis.Item = "DE"
						alertHis.Value = gnssData.E
						alertHis.Pre = preE
						handleAlert(&alertHis)

						alertHis.Item = "DN"
						alertHis.Value = gnssData.N
						alertHis.Pre = preN
						handleAlert(&alertHis)

						alertHis.Item = "DU"
						alertHis.Value = gnssData.U
						alertHis.Pre = preU
						handleAlert(&alertHis)

						alertHis.Pre = 0 //回归0
					}
				}

				var wsMsg interface{}
				dataPubSub.Range(func(ch, subs interface{}) bool {
					if _, ok := subs.(topicMap)[common.StationItemStruct{1, GNSSType}]; ok {
						if wsMsg == nil {
							wsMsg = struct {
								Sid       string `json:"sid"`
								Timestamp int64  `json:"timestamp"`
								enu
							}{gnssData.sid, gnssData.time.Unix(), gnssData.enu}
						}
						select {
						case ch.(chan interface{}) <- wsMsg:
						default:
							log.Println("------------------------------ channel已满 ------------------------------")
						}
					}
					return true
				})
				// 保存数据
				//dataHis.Sid = gnssData.sid
				//dataHis.Time = gnssData.time

				//if dataHis.Data.Set(gnssData.xyzenu) == nil {
				db.SaveDataHistory(1, gnssData.sid, "gnssData.xyzenu", gnssData.time)
				//}
				// pre
				preE = gnssData.E
				preN = gnssData.N
				preU = gnssData.U
			} //func recordDeliver

			for scanner.Scan() {
				ss := strings.Split(scanner.Text(), ",")
				if len(ss) != 10 || ss[8] != "1" {
					continue
				}

				gnssData.sid = ss[0]
				// 2019/04/24 00:54:25.000
				if gnssData.time, err = time.Parse("2006/01/02 15:04:05.000", ss[1]); err != nil {
					log.Println(err)
					continue
				}

				if gnssData.X, err = strconv.ParseFloat(ss[2], 64); err != nil {
					log.Println(err)
					continue
				}
				if gnssData.Y, err = strconv.ParseFloat(ss[3], 64); err != nil {
					log.Println(err)
					continue
				}
				if gnssData.Z, err = strconv.ParseFloat(ss[4], 64); err != nil {
					log.Println(err)
					continue
				}
				if gnssData.E, err = strconv.ParseFloat(ss[5], 64); err != nil {
					log.Println(err)
					continue
				}
				if gnssData.N, err = strconv.ParseFloat(ss[6], 64); err != nil {
					log.Println(err)
					continue
				}
				if gnssData.U, err = strconv.ParseFloat(ss[7], 64); err != nil {
					log.Println(err)
					continue
				}
				// https://phab.navi-tech.net/T72
				if math.Abs(gnssData.E) < 0.03 && math.Abs(gnssData.N) < 0.03 && math.Abs(gnssData.U) < 0.03 {
					tolerant = false
					biasedBuf = biasedBuf[:0]
					recordDeliver()
				} else {
					if gnssData.time.Unix()-preTs != 1 { //不连续就关闭容错模式, 并截断后append
						tolerant = false
						biasedBuf = append(biasedBuf[:0], gnssData)
					} else {
						if tolerant {
							recordDeliver()
						} else {
							if len(biasedBuf) == 4 {
								tolerant = true
								biasedBuf = append(biasedBuf, gnssData)
								for _, buf := range biasedBuf {
									//overwrite
									gnssData = buf
									recordDeliver()
								}
							} else {
								biasedBuf = append(biasedBuf, gnssData)
							}
						}
					}
				}
				preTs = gnssData.time.Unix() //过滤后更新preTs
			}
			if err := scanner.Err(); err != nil {
				if !strings.Contains(err.Error(), "use of closed network connection") {
					log.Println(err)
				}
			}
		}()
	}
}
