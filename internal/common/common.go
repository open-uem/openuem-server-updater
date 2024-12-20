package common

import (
	"context"

	"github.com/doncicuto/openuem-server-updater/internal/models"
	"github.com/doncicuto/openuem_utils"
	"github.com/go-co-op/gocron/v2"
	"github.com/nats-io/nats.go"
)

const NATS_TOKEN = "OpenUEM"

type UpdaterService struct {
	NATSConnection              *nats.Conn
	NATSConnectJob              gocron.Job
	DBConnectJob                gocron.Job
	DBUrl                       string
	NATSServers                 string
	CACert                      string
	UpdaterCert                 string
	UpdaterKey                  string
	Model                       *models.Model
	TaskScheduler               gocron.Scheduler
	Logger                      *openuem_utils.OpenUEMLogger
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
