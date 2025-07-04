package common

import (
	"context"
	"os"
	"strings"

	"github.com/open-uem/ent/server"
)

func (us *UpdaterService) SetInstalledComponents() error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	// Fix #1 hostname must not contain dots and domain
	hostnameParts := strings.Split(hostname, ".")
	hostname = hostnameParts[0]

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
