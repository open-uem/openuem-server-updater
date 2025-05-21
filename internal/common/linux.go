//go:build linux

package common

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/open-uem/ent/server"
	openuem_nats "github.com/open-uem/nats"
	"github.com/open-uem/utils"
	"github.com/zcalusic/sysinfo"
	"gopkg.in/ini.v1"
)

func NewUpdateService() (*UpdaterService, error) {
	var err error
	us := UpdaterService{}
	us.Logger = utils.NewLogger("openuem-server-updater")

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
	key, err = cfg.Section("Certificates").GetKey("UpdaterCert")
	if err != nil {
		log.Printf("[ERROR]: could not get updater cert path, reason: %v\n", err)
		return err
	}
	us.UpdaterCert = key.String()

	_, err = utils.ReadPEMCertificate(us.UpdaterCert)
	if err != nil {
		log.Fatalf("[FATAL]: could not read updater certificate")
	}

	key, err = cfg.Section("Certificates").GetKey("UpdaterKey")
	if err != nil {
		log.Printf("[ERROR]: could not get updater cert key, reason: %v\n", err)
		return err
	}
	us.UpdaterKey = key.String()

	_, err = utils.ReadPEMPrivateKey(us.UpdaterKey)
	if err != nil {
		log.Fatalf("[FATAL]: could not read updater private key")
	}

	key, err = cfg.Section("Certificates").GetKey("CACert")
	if err != nil {
		log.Printf("[ERROR]: could not get CA cert path, reason: %v\n", err)
		return err
	}

	us.CACert = key.String()
	_, err = utils.ReadPEMCertificate(us.CACert)
	if err != nil {
		log.Fatalf("[FATAL]: could not read CA certificate")
	}

	return nil
}

func (us *UpdaterService) ExecuteUpdate(data openuem_nats.OpenUEMUpdateRequest, msg jetstream.Msg, version string, channel server.Channel) {
	var cmd *exec.Cmd
	var err error

	operatingSystem := GetOSVendor()

	if err := msg.Ack(); err != nil {
		log.Printf("[ERROR]: could not ACK message, reason: %v", err)
		return
	}

	if err := us.Model.UpdateServerStatus(data.Version, channel, server.UpdateStatusInProgress, "", time.Now()); err != nil {
		log.Printf("[ERROR]: could not save server status, reason: %v", err)
	}

	switch operatingSystem {
	case "debian", "ubuntu", "linuxmint":
		cmd = exec.Command("/bin/sh", "-c", fmt.Sprintf("echo \"%s\" | at -M now +1 minute", "sudo apt update -y && sudo apt install -y --allow-downgrades openuem-server="+data.Version))
		err := cmd.Start()
		if err != nil {
			log.Printf("[ERROR]: could not run %s command, reason: %v", cmd.String(), err)
			return
		}
		log.Println("[INFO]: update command has been started: ", cmd.String())

		if err := cmd.Wait(); err != nil {
			log.Printf("[ERROR]: Command finished with error: %v", err)
			return
		}
		log.Println("[INFO]: update command has been programmed: ", cmd.String())
	case "fedora", "almalinux", "redhat", "rocky":
		packages := []string{}

		_, err = os.Stat("/opt/openuem-server/bin/openuem-nats-service")
		if err == nil {
			packages = append(packages, "openuem-nats-service-"+version)
		}
		_, err = os.Stat("/opt/openuem-server/bin/openuem-ocsp-responder")
		if err == nil {
			packages = append(packages, "openuem-ocsp-responder-"+version)
		}
		_, err = os.Stat("/opt/openuem-server/bin/openuem-agent-worker")
		if err == nil {
			packages = append(packages, "openuem-agent-worker-"+version)
		}
		_, err = os.Stat("/opt/openuem-server/bin/openuem-cert-manager-worker")
		if err == nil {
			packages = append(packages, "openuem-cert-manager-worker-"+version)
		}
		_, err = os.Stat("/opt/openuem-server/bin/openuem-notification-worker")
		if err == nil {
			packages = append(packages, "openuem-notification-worker-"+version)
		}
		_, err = os.Stat("/opt/openuem-server/bin/openuem-console")
		if err == nil {
			packages = append(packages, "openuem-console-"+version)
		}
		_, err = os.Stat("/opt/openuem-server/bin/openuem-server-updater")
		if err == nil {
			packages = append(packages, "openuem-server-updater-"+version)
		}
		_, err = os.Stat("/usr/bin/openuem-cert-manager")
		if err == nil {
			packages = append(packages, "openuem-cert-manager-"+version)
		}

		cmd = exec.Command("/bin/sh", "-c", fmt.Sprintf("echo \"sudo dnf install --allow-downgrade --refresh -y %s\" | at -M now +1 minute", strings.Join(packages, " ")))
		err := cmd.Start()
		if err != nil {
			log.Printf("[ERROR]: could not run %s command, reason: %v", cmd.String(), err)
			return
		}
		log.Println("[INFO]: update command has been started: ", cmd.String())

		if err := cmd.Wait(); err != nil {
			log.Printf("[ERROR]: Command finished with error: %v", err)
			return
		}
		log.Println("[INFO]: update command has been programmed: ", cmd.String())
	default:
		return
	}

}

func GetOSVendor() string {
	var si sysinfo.SysInfo

	si.GetSysInfo()

	return si.OS.Vendor
}
