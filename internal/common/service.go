package common

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/open-uem/ent/server"
	openuem_nats "github.com/open-uem/nats"
)

func (us *UpdaterService) StartService() {
	// Start the task scheduler
	us.TaskScheduler.Start()
	log.Println("[INFO]: task scheduler has been started")

	// Start DB connection job
	if err := us.StartDBConnectJob(); err != nil {
		return
	}

	// Start NATS connection job
	if err := us.StartNATSConnectJob(us.queueSubscribe); err != nil {
		return
	}
}

func (us *UpdaterService) StopService() {
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

func (us *UpdaterService) queueSubscribe() error {
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

	serverStreamConfig := jetstream.StreamConfig{
		Name:      "SERVERS_STREAM",
		Subjects:  []string{"server.update.>"},
		Retention: jetstream.InterestPolicy,
	}

	replicas := strings.Split(us.NATSServers, ",")

	if len(replicas) > 1 {
		serverStreamConfig.Replicas = len(replicas)
	}

	s, err := js.CreateOrUpdateStream(ctx, serverStreamConfig)
	if err != nil {
		log.Printf("[ERROR]: could not instantiate SERVERS_STREAM, reason: %v\n", err)
		return err
	}

	consumerConfig := jetstream.ConsumerConfig{
		Durable:        "ServerUpdater" + hostname,
		AckWait:        10 * time.Minute,
		AckPolicy:      jetstream.AckExplicitPolicy,
		FilterSubjects: []string{"server.update." + hostname},
	}

	if len(strings.Split(us.NATSServers, ",")) > 1 {
		consumerConfig.Replicas = int(math.Min(float64(len(strings.Split(us.NATSServers, ","))), 5))
	}

	c1, err := s.CreateOrUpdateConsumer(ctx, consumerConfig)
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
		log.Printf("[ERROR]: could not unmarshal update request, reason: %v\n", err)

		if err := msg.Ack(); err != nil {
			log.Printf("[ERROR]: could not ACK message, reason: %v", err)
			return
		}
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
			log.Printf("[ERROR]: could not schedule the update task: %v\n", err)

			if err := msg.Ack(); err != nil {
				log.Printf("[ERROR]: could not ACK message, reason: %v", err)
			}

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
				log.Printf("[ERROR]: could not schedule the update task: %v\n", err)

				if err := msg.Ack(); err != nil {
					log.Printf("[ERROR]: could not ACK message, reason: %v", err)
					return
				}

				if err := us.Model.UpdateServerStatus(data.Version, channel, server.UpdateStatusError, fmt.Sprintf("could not schedule the update task: %v", err), time.Now()); err != nil {
					log.Printf("[ERROR]: could not save update server status: %v\n", err)
				}
				return
			}
			log.Printf("[INFO]: new update task scheduled a %s", data.UpdateAt.String())
		}
	}
}
