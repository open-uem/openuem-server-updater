package common

import (
	"log"
	"time"

	"github.com/doncicuto/openuem-server-updater/internal/models"
	"github.com/doncicuto/openuem_ent/server"
	"github.com/go-co-op/gocron/v2"
)

func (us *UpdaterService) StartDBConnectJob(version string) error {
	var err error

	us.Model, err = models.New(us.DBUrl)
	if err == nil {
		log.Println("[INFO]: connection established with database")

		if err := us.SetServer(); err != nil {
			log.Fatalf("[FATAL]: %v", err)
		}

		if err := us.SetInstalledComponents(); err != nil {
			log.Fatalf("[FATAL]: %v", err)
		}

		// Evaluate result of previous update
		s, err := us.Model.GetServerStatus()
		if err != nil {
			log.Println("[ERROR]: could not get server status")
		}

		if s.UpdateStatus == server.UpdateStatusInProgress && s.Version == version {
			if err := us.Model.UpdateServerStatus(s.Version, s.Channel, server.UpdateStatusSuccess, s.UpdateMessage, s.UpdateWhen); err != nil {
				log.Printf("[ERROR]: could not save server status, reason: %v\n", err)
			}
		}
		return nil
	}
	log.Printf("[ERROR]: could not connect with database %v", err)

	// Create task
	us.DBConnectJob, err = us.TaskScheduler.NewJob(
		gocron.DurationJob(
			time.Duration(time.Duration(30*time.Second)),
		),
		gocron.NewTask(
			func() {
				us.Model, err = models.New(us.DBUrl)
				if err != nil {
					log.Printf("[ERROR]: could not connect with database %v", err)
					return
				}
				log.Println("[INFO]: connection established with database")
				if err := us.TaskScheduler.RemoveJob(us.DBConnectJob.ID()); err != nil {
					return
				}

				if err := us.SetServer(); err != nil {
					log.Fatalf("[FATAL]: %v", err)
				}

				if err := us.SetInstalledComponents(); err != nil {
					log.Fatalf("[FATAL]: %v", err)
				}

				// Evaluate result of previous update
				s, err := us.Model.GetServerStatus()
				if err != nil {
					log.Println("[ERROR]: could not get server status")
				}

				if s.UpdateStatus == server.UpdateStatusInProgress && s.Version == version {
					if err := us.Model.UpdateServerStatus(s.Version, s.Channel, server.UpdateStatusSuccess, s.UpdateMessage, s.UpdateWhen); err != nil {
						log.Printf("[ERROR]: could not save server status, reason: %v\n", err)
					}
				}
			},
		),
	)
	if err != nil {
		log.Fatalf("[FATAL]: could not start the DB connect job: %v", err)
		return err
	}
	log.Printf("[INFO]: new DB connect job has been scheduled every %d minutes", 2)
	return nil
}
