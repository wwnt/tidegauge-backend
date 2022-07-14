package project

import (
	"golang.org/x/sys/windows/svc"
	"log"
)

//https://docs.microsoft.com/en-us/windows/win32/services/service-control-handler-function
type service struct {
	startCallback    func()
	stopCallback     func()
	shutdownCallback func()
}

func (ms *service) Execute(_ []string, req <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdAccepted = svc.AcceptStop | svc.AcceptPreShutdown
	changes <- svc.Status{State: svc.StartPending}
	ms.startCallback()
	changes <- svc.Status{State: svc.Running, Accepts: cmdAccepted}
loop:
	for c := range req {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop:
			ms.stopCallback()
			break loop
		case svc.Shutdown:
			ms.shutdownCallback()
			break loop
		case svc.PreShutdown:
			ms.shutdownCallback()
			break loop
		default:
			log.Printf("unexpected control request #%d", c)
		}
	}
	changes <- svc.Status{State: svc.StopPending}
	return
}
func runService(start, stop, shutdown func()) {
	inService, err := svc.IsWindowsService()
	if err != nil {
		log.Printf("failed to determine if we are running in service: %v", err)
		return
	}
	if !inService {
		log.Println("not in service")
		return
	}
	err = svc.Run("tide", &service{start, stop, shutdown})
	if err != nil {
		log.Println("service failed:", err)
	}
}
