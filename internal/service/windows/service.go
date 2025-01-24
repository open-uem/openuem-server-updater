//go:build windows

package main

import (
	"log"

	"github.com/open-uem/openuem-server-updater/internal/common"
	"github.com/open-uem/utils"
	"golang.org/x/sys/windows/svc"
)

func main() {
	us, err := common.NewUpdateService()
	if err != nil {
		log.Fatalf("[FATAL]: could not create task scheduler, reason: %s", err.Error())
	}

	if err := us.ReadConfig(); err != nil {
		log.Printf("[ERROR]: %v", err)
		if err := us.StartReadConfigJob(); err != nil {
			log.Fatalf("[FATAL]: %v", err)
		}
	}

	ws := utils.NewOpenUEMWindowsService()
	ws.ServiceStart = us.StartService
	ws.ServiceStop = us.StopService

	// Run service
	err = svc.Run("openuem-updater-service", ws)
	if err != nil {
		log.Printf("[ERROR]: could not run service: %v", err)
	}
}
