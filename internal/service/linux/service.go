//go:build linux

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/open-uem/openuem-server-updater/internal/common"
)

func main() {
	us, err := common.NewUpdateService()
	if err != nil {
		log.Fatalf("[FATAL]: could not create task scheduler, reason: %s", err.Error())
	}

	if err := us.ReadConfig(); err != nil {
		log.Printf("[ERROR]: %v", err)
		if err := us.StartReadConfigJob(); err != nil {
			log.Printf("[FATAL]: %v", err)
		}
	}

	us.StartService()

	// Keep the connection alive
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	log.Println("[INFO]: the server updater service has started")
	<-done

	us.StopService()
}
