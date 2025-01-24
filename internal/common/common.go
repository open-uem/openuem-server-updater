package common

import (
	"context"

	"github.com/go-co-op/gocron/v2"
	"github.com/nats-io/nats.go"
	"github.com/open-uem/openuem-server-updater/internal/models"
	"github.com/open-uem/utils"
)

const NATS_TOKEN = "OpenUEM"

type UpdaterService struct {
	NATSConnection              *nats.Conn
	NATSConnectJob              gocron.Job
	DBConnectJob                gocron.Job
	ConfigJob                   gocron.Job
	DBUrl                       string
	NATSServers                 string
	CACert                      string
	UpdaterCert                 string
	UpdaterKey                  string
	Model                       *models.Model
	TaskScheduler               gocron.Scheduler
	Logger                      *utils.OpenUEMLogger
	JetstreamContextCancel      context.CancelFunc
	NATSInstalled               bool
	ConsoleInstalled            bool
	AgentWorkerInstalled        bool
	NotificationWorkerInstalled bool
	CertManagerWorkerInstalled  bool
	OCSPResponderInstalled      bool
	Version                     string
	Channel                     string
}
