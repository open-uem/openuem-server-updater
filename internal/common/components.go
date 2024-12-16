package common

import (
	"context"
	"os"

	"github.com/doncicuto/openuem_ent/server"
)

func (us *UpdaterService) SetInstalledComponents() error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	return us.Model.Client.Server.Update().
		SetOcspComponent(us.OCSPResponderInstalled).
		SetNatsComponent(us.NATSInstalled).
		SetConsoleComponent(us.ConsoleInstalled).
		SetAgentWorkerComponent(us.AgentWorkerInstalled).
		SetCertManagerWorkerComponent(us.CertManagerWorkerInstalled).
		SetNotificationWorkerComponent(us.NotificationWorkerInstalled).
		Where(server.Hostname(hostname)).
		Exec(context.Background())
}
