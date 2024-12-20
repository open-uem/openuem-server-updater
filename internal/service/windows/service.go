//go:build windows

package main

import (
	"log"

	"github.com/doncicuto/openuem-server-updater/internal/common"
	"github.com/doncicuto/openuem_utils"
	"golang.org/x/sys/windows/svc"
)

func main() {
	us, err := common.NewUpdateService()
	if err != nil {
		log.Fatalf("[FATAL]: could not create task scheduler, reason: %s", err.Error())
	}

	if err := us.ReadConfig(); err != nil {
		log.Fatalf("[FATAL]: %v", err)
	}

	ws := openuem_utils.NewOpenUEMWindowsService()
	ws.ServiceStart = us.StartService
	ws.ServiceStop = us.StopService

	// Run service
	err = svc.Run("openuem-updater-service", ws)
	if err != nil {
		log.Printf("[ERROR]: could not run service: %v", err)
	}
}
