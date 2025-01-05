//go:build windows

package common

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/open-uem/ent/server"
	openuem_nats "github.com/open-uem/nats"
	"github.com/open-uem/utils"
	"gopkg.in/ini.v1"
)

func NewUpdateService() (*UpdaterService, error) {
	var err error
	us := UpdaterService{}
	us.Logger = utils.NewLogger("openuem-server-updater.txt")

	us.TaskScheduler, err = gocron.NewScheduler()
	if err != nil {
		return nil, err
	}

	return &us, nil
}

func (us *UpdaterService) ReadConfig() error {
	var err error

	// Get conf file
	configFile := utils.GetConfigFile()

	// Open ini file
	cfg, err := ini.Load(configFile)
	if err != nil {
		return err
	}

	key, err := cfg.Section("NATS").GetKey("NATSServers")
	if err != nil {
		log.Println("[ERROR]: could not get NATSServers")
		return err
	}
	us.NATSServers = key.String()

	key, err = cfg.Section("Server").GetKey("Version")
	if err != nil {
		log.Println("[ERROR]: could not get Version")
		return err
	}
	us.Version = key.String()

	key, err = cfg.Section("Server").GetKey("Channel")
	if err != nil {
		log.Println("[ERROR]: could not get Chanel")
		return err
	}
	us.Channel = key.String()

	key, err = cfg.Section("Components").GetKey("OCSP")
	if err == nil {
		if key.String() == "yes" {
			us.OCSPResponderInstalled = true
		}
	}

	key, err = cfg.Section("Components").GetKey("NATS")
	if err == nil {
		if key.String() == "yes" {
			us.NATSInstalled = true
		}
	}

	key, err = cfg.Section("Components").GetKey("AgentWorker")
	if err == nil {
		if key.String() == "yes" {
			us.AgentWorkerInstalled = true
		}
	}

	key, err = cfg.Section("Components").GetKey("CertManagerWorker")
	if err == nil {
		if key.String() == "yes" {
			us.CertManagerWorkerInstalled = true
		}
	}

	key, err = cfg.Section("Components").GetKey("NotificationWorker")
	if err == nil {
		if key.String() == "yes" {
			us.NotificationWorkerInstalled = true
		}
	}

	key, err = cfg.Section("Components").GetKey("Console")
	if err == nil {
		if key.String() == "yes" {
			us.ConsoleInstalled = true
		}
	}

	// Read the DBUrl
	us.DBUrl, err = utils.CreatePostgresDatabaseURL()
	if err != nil {
		log.Printf("[ERROR]: could not get database url, reason: %v\n", err)
		return err
	}

	// Read required certificates and private key
	cwd, err := utils.GetWd()
	if err != nil {
		log.Fatalf("[FATAL]: could not get current working directory")
	}

	us.UpdaterCert = filepath.Join(cwd, "certificates", "updater", "updater.cer")
	_, err = utils.ReadPEMCertificate(us.UpdaterCert)
	if err != nil {
		log.Fatalf("[FATAL]: could not read updater certificate")
	}

	us.UpdaterKey = filepath.Join(cwd, "certificates", "updater", "updater.key")
	_, err = utils.ReadPEMPrivateKey(us.UpdaterKey)
	if err != nil {
		log.Fatalf("[FATAL]: could not read updater private key")
	}

	us.CACert = filepath.Join(cwd, "certificates", "ca", "ca.cer")
	_, err = utils.ReadPEMCertificate(us.CACert)
	if err != nil {
		log.Fatalf("[FATAL]: could not read CA certificate")
	}

	return nil
}

func (us *UpdaterService) ExecuteUpdate(data openuem_nats.OpenUEMUpdateRequest, msg jetstream.Msg, version string, channel server.Channel) {
	// Download the file
	cwd, err := utils.GetWd()
	if err != nil {
		log.Printf("[ERROR]: could not get working directory, reason %v", err)
		msg.Ack()
		if err := us.Model.UpdateServerStatus(data.Version, channel, server.UpdateStatusError, fmt.Sprintf("could not get working directory, reason: %v", err), time.Now()); err != nil {
			log.Printf("[ERROR]: could not save server status, reason: %v", err)
		}
		return
	}

	downloadPath := filepath.Join(cwd, "updates", "server-setup.exe")
	if err := utils.DownloadFile(data.DownloadFrom, downloadPath, data.DownloadHash); err != nil {
		log.Printf("[ERROR]: could not download update to directory, reason %v", err)
		msg.NakWithDelay(60 * time.Minute)
		if err := us.Model.UpdateServerStatus(data.Version, channel, server.UpdateStatusError, fmt.Sprintf("could not download update to directory, reason: %v", err), time.Now()); err != nil {
			log.Printf("[ERROR]: could not save server status, reason: %v", err)
		}
		return
	}

	msg.Ack()
	if err := us.Model.UpdateServerStatus(data.Version, channel, server.UpdateStatusInProgress, "", time.Now()); err != nil {
		log.Printf("[ERROR]: could not save server status, reason: %v", err)
	}
	cmd := exec.Command(downloadPath, "/VERYSILENT")
	err = cmd.Start()
	if err != nil {
		log.Printf("[ERROR]: could not run %s command, reason: %v", downloadPath, err)
		return
	}
}
