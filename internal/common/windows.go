//go:build windows

package common

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/doncicuto/openuem_ent/server"
	"github.com/doncicuto/openuem_nats"
	"github.com/doncicuto/openuem_utils"
	"github.com/go-co-op/gocron/v2"
	"github.com/nats-io/nats.go/jetstream"
	"gopkg.in/ini.v1"
)

func (us *UpdaterService) StartWindowsService() {
	// Start the task scheduler
	us.TaskScheduler.Start()
	log.Println("[INFO]: task scheduler has been started")

	// Start DB connection job
	if err := us.StartDBConnectJob(VERSION); err != nil {
		return
	}

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

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	js, err := jetstream.New(us.NATSConnection)
	if err != nil {
		log.Printf("[ERROR]: could not instantiate JetStream: %s", err.Error())
		return err
	}
	log.Println("[INFO]: JetStream has been instantiated")

	ctx, us.JetstreamContextCancel = context.WithTimeout(context.Background(), 60*time.Minute)
	s, err := js.Stream(ctx, "SERVERS_STREAM")
	if err != nil {
		log.Printf("[ERROR]: could not instantiate SERVERS_STREAM, reason: %v\n", err)
		return err
	}

	c1, err := s.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:        "ServerUpdater" + hostname,
		AckWait:        10 * time.Minute,
		AckPolicy:      jetstream.AckExplicitPolicy,
		FilterSubjects: []string{"server.update." + hostname},
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

	return nil
}

func (us *UpdaterService) JetStreamUpdaterHandler(msg jetstream.Msg) {
	var channel server.Channel
	data := openuem_nats.OpenUEMUpdateRequest{}

	if err := json.Unmarshal(msg.Data(), &data); err != nil {
		msg.Ack()
		log.Printf("[ERROR]: could not unmarshal update request, reason: %v\n", err)
		return
	}

	switch data.Channel {
	case "stable":
		channel = server.ChannelStable
	case "devel":
		channel = server.ChannelDevel
	case "testing":
		channel = server.ChannelTesting
	default:
		channel = server.ChannelStable
	}

	// If scheduled time is in the past execute now
	if !time.Time.IsZero(data.UpdateAt) && data.UpdateAt.Before(time.Now().Local()) {
		data.UpdateNow = true
	}

	if data.UpdateNow {
		_, err := us.TaskScheduler.NewJob(
			gocron.OneTimeJob(
				gocron.OneTimeJobStartImmediately(),
			),
			gocron.NewTask(
				func() {
					us.ExecuteUpdate(data, msg, data.Version, channel)
				},
			),
		)

		if err != nil {
			msg.Ack()
			log.Printf("[ERROR]: could not schedule the update task: %v\n", err)
			if err := us.Model.UpdateServerStatus(data.Version, channel, server.UpdateStatusError, fmt.Sprintf("could not schedule the update task: %v", err), time.Now()); err != nil {
				log.Printf("[ERROR]: could not save update server status: %v\n", err)
			}
			return
		}
		log.Println("[INFO]: new update task will run now")
	} else {
		if !time.Time.IsZero(data.UpdateAt) {
			_, err := us.TaskScheduler.NewJob(
				gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(data.UpdateAt)),
				gocron.NewTask(func() {
					us.ExecuteUpdate(data, msg, data.Version, channel)
				}),
			)

			if err != nil {
				msg.Ack()
				log.Printf("[ERROR]: could not schedule the update task: %v\n", err)
				if err := us.Model.UpdateServerStatus(data.Version, channel, server.UpdateStatusError, fmt.Sprintf("could not schedule the update task: %v", err), time.Now()); err != nil {
					log.Printf("[ERROR]: could not save update server status: %v\n", err)
				}
				return
			}
			log.Printf("[INFO]: new update task scheduled a %s", data.UpdateAt.String())
		}
	}
}

func (us *UpdaterService) ExecuteUpdate(data openuem_nats.OpenUEMUpdateRequest, msg jetstream.Msg, version string, channel server.Channel) {
	// Download the file
	cwd, err := openuem_utils.GetWd()
	if err != nil {
		log.Printf("[ERROR]: could not get working directory, reason %v", err)
		msg.Ack()
		if err := us.Model.UpdateServerStatus(data.Version, channel, server.UpdateStatusError, fmt.Sprintf("could not get working directory, reason: %v", err), time.Now()); err != nil {
			log.Printf("[ERROR]: could not save server status, reason: %v", err)
		}
		return
	}

	downloadPath := filepath.Join(cwd, "updates", "server-setup.exe")
	if err := openuem_utils.DownloadFile(data.DownloadFrom, downloadPath, data.DownloadHash); err != nil {
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

func (us *UpdaterService) ReadWindowsConfig() error {
	var err error

	// Get conf file
	configFile := openuem_utils.GetConfigFile()

	// Open ini file
	cfg, err := ini.Load(configFile)
	if err != nil {
		return err
	}

	key, err := cfg.Section("NATS").GetKey("NATSServer")
	if err != nil {
		log.Println("[ERROR]: could not get NATSServers")
		return err
	}
	us.NATSServers = key.String()

	// Read the DBUrl
	us.DBUrl, err = openuem_utils.CreatePostgresDatabaseURL()
	if err != nil {
		log.Printf("[ERROR]: could not get database url, reason: %v\n", err)
		return err
	}

	// Read required certificates and private key
	cwd, err := openuem_utils.GetWd()
	if err != nil {
		log.Fatalf("[FATAL]: could not get current working directory")
	}

	us.UpdaterCert = filepath.Join(cwd, "certificates", "updater", "updater.cer")
	_, err = openuem_utils.ReadPEMCertificate(us.UpdaterCert)
	if err != nil {
		log.Fatalf("[FATAL]: could not read updater certificate")
	}

	us.UpdaterKey = filepath.Join(cwd, "certificates", "updater", "updater.key")
	_, err = openuem_utils.ReadPEMPrivateKey(us.UpdaterKey)
	if err != nil {
		log.Fatalf("[FATAL]: could not read updater private key")
	}

	us.CACert = filepath.Join(cwd, "certificates", "ca", "ca.cer")
	_, err = openuem_utils.ReadPEMCertificate(us.CACert)
	if err != nil {
		log.Fatalf("[FATAL]: could not read CA certificate")
	}

	return nil
}
