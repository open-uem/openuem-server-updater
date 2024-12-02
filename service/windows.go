//go:build windows

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/doncicuto/openuem_nats"
	"github.com/doncicuto/openuem_utils"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
)

func (us *UpdaterService) StartWindowsService() {
	// Start the task scheduler
	us.TaskScheduler.Start()
	log.Println("[INFO]: task scheduler has been started")

	// Start NATS connection job
	if err := us.StartNATSConnectJob(us.queueSubscribeForWindows); err != nil {
		return
	}
}

func (us *UpdaterService) StopWindowsService() {
	if us.Logger != nil {
		us.Logger.Close()
	}

	if us.NATSConnection != nil {
		if err := us.NATSConnection.Flush(); err != nil {
			log.Println("[ERROR]: could not flush NATS connection")
		}
		us.NATSConnection.Close()
	}
}

func (us *UpdaterService) queueSubscribeForWindows() error {
	var ctx context.Context

	js, err := jetstream.New(us.NATSConnection)
	if err != nil {
		log.Printf("[ERROR]: could not instantiate JetStream: %s", err.Error())
		return err
	}
	log.Println("[INFO]: JetStream has been instantiated")

	ctx, us.JetstreamContextCancel = context.WithTimeout(context.Background(), 60*time.Minute)
	s, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     "SERVER_UPDATER_STREAM_" + us.UUID,
		Subjects: []string{"server.update." + us.UUID, "server.rollback." + us.UUID},
	})
	if err != nil {
		log.Printf("[ERROR]: could not create stream SERVER_UPDATER_STREAM_%s: %v\n", us.UUID, err)
		return err
	}
	log.Printf("[INFO]: SERVER_UPDATER_STREAM_%s stream has been created or updated", us.UUID)

	c1, err := s.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		AckPolicy: jetstream.AckExplicitPolicy,
	})
	if err != nil {
		log.Printf("[ERROR]: could not create Jetstream consumer: %s", err.Error())
		return err
	}
	// TODO stop consume context ()
	_, err = c1.Consume(us.JetStreamUpdaterHandler, jetstream.ConsumeErrHandler(func(consumeCtx jetstream.ConsumeContext, err error) {
		log.Printf("[ERROR]: consumer error: %s", err.Error())
	}))

	log.Println("[INFO]: Jetstream created and started consuming messages")
	log.Println("[INFO]: subscribed to message ", "server.update."+us.UUID)
	log.Println("[INFO]: subscribed to message ", "server.rollback."+us.UUID)

	return nil
}

func (us *UpdaterService) JetStreamUpdaterHandler(msg jetstream.Msg) {
	// Unmarshal msg data
	data := openuem_nats.OpenUEMServerRelease{}
	if err := json.Unmarshal(msg.Data(), &data); err != nil {
		log.Printf("[ERROR]: could not unmarshal update request, reason: %v\n", err)
		msg.NakWithDelay(60 * time.Minute)
		return
	}

	if msg.Subject() == fmt.Sprintf("server.update.%s", us.UUID) {
		if us.HasComponent("nats") {
			us.ComponentUpdate(data, "openuem-nats-service")
		}

		if us.HasComponent("ocsp") {
			us.ComponentUpdate(data, "openuem-ocsp-responder")
		}

		if us.HasComponent("console") {
			us.ComponentUpdate(data, "openuem-console-service")
		}

		if us.HasComponent("agent-worker") {
			us.ComponentUpdate(data, "openuem-agent-worker")
		}

		if us.HasComponent("notification-worker") {
			us.ComponentUpdate(data, "openuem-notification-worker")
		}

		if us.HasComponent("cert-manager-worker") {
			us.ComponentUpdate(data, "openuem-cert-manager-worker")
		}

		if us.HasComponent("cert-manager") {
			us.CertManagerUpdate(data)
		}
	}

	if msg.Subject() == fmt.Sprintf("server.rollback.%s", us.UUID) {
		if us.HasComponent("nats") {
			us.ComponentRollback("openuem-nats-service")
		}

		if us.HasComponent("ocsp") {
			us.ComponentRollback("openuem-ocsp-responder")
		}

		if us.HasComponent("console") {
			us.ComponentRollback("openuem-console-service")
		}

		if us.HasComponent("agent-worker") {
			us.ComponentRollback("openuem-agent-worker")
		}

		if us.HasComponent("notification-worker") {
			us.ComponentRollback("openuem-notification-worker")
		}

		if us.HasComponent("cert-manager-worker") {
			us.ComponentRollback("openuem-cert-manager-worker")
		}

		if us.HasComponent("cert-manager") {
			us.CertManagerRollback()
		}
	}
}

func (us *UpdaterService) ReadWindowsConfig() error {
	var err error

	k, err := openuem_utils.OpenRegistryForQuery(registry.LOCAL_MACHINE, `SOFTWARE\OpenUEM\Server`)
	if err != nil {
		log.Println("[ERROR]: could not open registry")
		return err
	}
	defer k.Close()

	us.NATSServers, err = openuem_utils.GetValueFromRegistry(k, "NATSServers")
	if err != nil {
		return fmt.Errorf("could not read NATS servers from registry")
	}

	us.UUID, err = openuem_utils.GetValueFromRegistry(k, "UUID")
	if err != nil {
		return fmt.Errorf("could not read NATS servers from registry")
	}

	us.DBUrl, err = openuem_utils.CreatePostgresDatabaseURL()
	if err != nil {
		log.Printf("[ERROR]: %v", err)
		return err
	}
	k.Close()

	k, err = registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\OpenUEM\Server`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		log.Println("[ERROR]: cannot read the server hive")
		return err
	}
	defer k.Close()

	if us.UUID == "" {
		k.SetStringValue("UUID", uuid.NewString())
	}

	return nil
}

func (us *UpdaterService) ComponentUpdate(data openuem_nats.OpenUEMServerRelease, component string) error {

	// Download the file
	cwd, err := openuem_utils.GetWd()
	if err != nil {
		return err
	}

	componentInfo, ok := data.Files[component]
	if !ok {
		return fmt.Errorf("component release info not found")
	}

	fileInfo := openuem_nats.ServerFileInfo{}
	for _, item := range componentInfo {
		if item.Arch == runtime.GOARCH && item.Os == runtime.GOOS {
			fileInfo.FileURL = item.FileURL
			fileInfo.Checksum = item.Checksum
			break
		}
	}

	if fileInfo.FileURL == "" || fileInfo.Checksum == "" {
		return fmt.Errorf("component file info not found")
	}

	downloadPath := filepath.Join(cwd, "updater", component+"-download.exe")
	if err := openuem_utils.DownloadFile(fileInfo.FileURL, downloadPath, fileInfo.Checksum); err != nil {
		return err
	}

	// Stop service
	if err := openuem_utils.WindowsSvcControl(component, svc.Stop, svc.Stopped); err != nil {
		return err
	}

	// Preparing for rollback
	exePath := filepath.Join(cwd, component+".exe")
	exeWasFound := true
	if _, err := os.Stat(exePath); err != nil {
		log.Printf("[WARN]: could not find previous %s, reason %v", component, err)
		exeWasFound = false
	}

	if exeWasFound {
		// Rename old component
		rollbackPath := filepath.Join(cwd, "updater", component+"-rollback.exe")
		if err := os.Rename(exePath, rollbackPath); err != nil {
			return err
		}
	}

	// Rename downloaded component as the new exe
	if err := os.Rename(downloadPath, exePath); err != nil {
		return err
	}

	// Start service
	if err := openuem_utils.WindowsStartService(component); err != nil {
		// We couldn't start service maybe we should rollback
		// but only if we had a previous exe
		if exeWasFound {
			rollbackPath := filepath.Join(cwd, component+".exe")
			if err := os.Rename(rollbackPath, exePath); err != nil {
				return err
			}

			// try to start this exe now
			if err := openuem_utils.WindowsStartService(component); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	log.Printf("[INFO]: %s was installed and started", component)
	return nil
}

func (us *UpdaterService) ComponentRollback(component string) error {
	cwd, err := openuem_utils.GetWd()
	if err != nil {
		return err
	}

	rollbackPath := filepath.Join(cwd, "updater", component+"-rollback.exe")
	exePath := filepath.Join(cwd, component+".exe")

	// Preparing for rollback
	if _, err := os.Stat(rollbackPath); err != nil {
		return err
	}

	// Stop service
	if err := openuem_utils.WindowsSvcControl(component, svc.Stop, svc.Stopped); err != nil {
		return err
	}

	// Rename rollback
	if err := os.Rename(rollbackPath, exePath); err != nil {
		return err
	}

	// Start service
	if err := openuem_utils.WindowsStartService(component); err != nil {
		return err
	}

	log.Printf("[INFO]: %s was rolled back", component)
	return nil
}

func (us *UpdaterService) CertManagerUpdate(data openuem_nats.OpenUEMServerRelease) error {
	// Download the file
	cwd, err := openuem_utils.GetWd()
	if err != nil {
		return err
	}

	componentInfo, ok := data.Files["cert-manager"]
	if !ok {
		return fmt.Errorf("cert-manager release info not found")
	}

	fileInfo := openuem_nats.ServerFileInfo{}
	for _, item := range componentInfo {
		if item.Arch == runtime.GOARCH && item.Os == runtime.GOOS {
			fileInfo.FileURL = item.FileURL
			fileInfo.Checksum = item.Checksum
			break
		}
	}

	if fileInfo.FileURL == "" || fileInfo.Checksum == "" {
		return fmt.Errorf("cert-manager file info not found")
	}

	downloadPath := filepath.Join(cwd, "updater", "openuem-cert-manager-download.exe")
	if err := openuem_utils.DownloadFile(fileInfo.FileURL, downloadPath, fileInfo.Checksum); err != nil {
		return err
	}

	// Preparing for rollback
	exePath := filepath.Join(cwd, "openuem-cert-manager.exe")
	exeWasFound := true
	if _, err := os.Stat(exePath); err != nil {
		log.Printf("[WARN]: could not find previous %s, reason %v", "openuem-cert-manager", err)
		exeWasFound = false
	}

	if exeWasFound {
		// Rename old component
		rollbackPath := filepath.Join(cwd, "updater", "openuem-cert-manager-rollback.exe")
		if err := os.Rename(exePath, rollbackPath); err != nil {
			return err
		}
	}

	// Rename downloaded component as the new exe
	if err := os.Rename(downloadPath, exePath); err != nil {
		return err
	}

	log.Printf("[INFO]: %s was installed and started", "openuem-cert-manager")
	return nil
}

func (us *UpdaterService) CertManagerRollback() error {
	cwd, err := openuem_utils.GetWd()
	if err != nil {
		return err
	}

	rollbackPath := filepath.Join(cwd, "updater", "openuem-cert-manager-rollback.exe")
	exePath := filepath.Join(cwd, "openuem-cert-manager.exe")

	// Preparing for rollback
	if _, err := os.Stat(rollbackPath); err != nil {
		return err
	}

	// Rename rollback
	if err := os.Rename(rollbackPath, exePath); err != nil {
		return err
	}

	log.Printf("[INFO]: %s was rolled back", "openuem-cert-manager")
	return nil
}

func (us *UpdaterService) HasComponent(name string) bool {
	cwd, err := openuem_utils.GetWd()
	if err != nil {
		log.Println("[ERROR]: could not get current working directory")
		return false
	}

	path := ""
	switch name {
	case "agent":
		path = filepath.Join(cwd, "openuem-agent.exe")
	case "nats":
		path = filepath.Join(cwd, "openuem-nats-service.exe")
	case "ocsp":
		path = filepath.Join(cwd, "openuem-ocsp-responder.exe")
	case "agent-worker":
		path = filepath.Join(cwd, "openuem-agent-worker.exe")
	case "notification-worker":
		path = filepath.Join(cwd, "openuem-notification-worker.exe")
	case "cert-manager-worker":
		path = filepath.Join(cwd, "openuem-cert-manager-worker.exe")
	case "console":
		path = filepath.Join(cwd, "openuem-console-service.exe")
	case "cert-manager":
		path = filepath.Join(cwd, "openuem-cert-manager.exe")
	}

	_, err = os.Stat(path)
	if err != nil {
		return false
	}
	return true
}
