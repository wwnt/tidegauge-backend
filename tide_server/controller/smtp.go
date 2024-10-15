package controller

import (
	"net/smtp"
	"tide/tide_server/global"
)

func SendMail(to []string, body string) error {
	if global.Smtp.Auth != nil && len(to) > 0 {
		msg := []byte("From: " + global.Config.Smtp.Username + "\r\nSubject: TideGauge Account\r\n\r\n" + body + "\r\n")
		err := smtp.SendMail(global.Config.Smtp.Addr, global.Smtp.Auth, global.Config.Smtp.Username, to, msg)
		return err
	}
	return nil
}

func mailDelUser(username string, addr string) {
	if err := SendMail([]string{addr}, "Account: "+username+" has been deleted."); err != nil {
		logger.Warn(err.Error())
	}
}
