package common

const (
	NoStatus     Status = "NoStatus"
	Disconnected Status = "Disconnected"
	Normal       Status = "Normal"
	Abnormal     Status = "Abnormal"
)

const (
	MsgData           MsgType = 0
	MsgGpioData       MsgType = 1
	MsgRpiStatus      MsgType = 2
	MsgItemStatus     MsgType = 3
	MsgCameraSnapShot MsgType = 5
)
