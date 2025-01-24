package common

import (
	"log"
	"time"

	"github.com/go-co-op/gocron/v2"
)

func (us *UpdaterService) StartReadConfigJob() error {
	var err error

	// Create task for getting the worker config
	us.ConfigJob, err = us.TaskScheduler.NewJob(
		gocron.DurationJob(
			time.Duration(time.Duration(1*time.Minute)),
		),
		gocron.NewTask(
			func() {
				err = us.ReadConfig()
				if err != nil {
					log.Printf("[ERROR]: could not generate config for server updater, reason: %v", err)
					return
				}

				log.Println("[INFO]: server updater's config has been successfully generated")
				if err := us.TaskScheduler.RemoveJob(us.ConfigJob.ID()); err != nil {
					return
				}
				return
			},
		),
	)
	if err != nil {
		log.Fatalf("[FATAL]: could not start the read server updater config job: %v", err)
		return err
	}
	log.Printf("[INFO]: new read server updater config job has been scheduled every %d minute", 1)
	return nil
}
