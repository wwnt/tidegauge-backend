package common

const (
	NoStatus     Status = "NoStatus"
	Disconnected Status = "Disconnected"
	Normal       Status = "Normal"
	Abnormal     Status = "Abnormal"
)

const (
	MsgData MsgType = iota
	MsgGpioData
	MsgRpiStatus
	MsgItemStatus
	MsgPortTerminal
	MsgCameraSnapShot
)
