package models

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"

	openuem_ent "github.com/open-uem/ent"
	"github.com/open-uem/ent/server"
)

func (m *Model) SetServer(version string, channel server.Channel) error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	// Fix #1 hostname must not contain dots and domain
	hostnameParts := strings.Split(hostname, ".")
	hostname = hostnameParts[0]

	exists := true
	s, err := m.Client.Server.Query().Where(server.Hostname(hostname), server.Arch(runtime.GOARCH), server.Os(runtime.GOOS), server.ChannelEQ(channel)).Only(context.Background())
	if err != nil {
		if !openuem_ent.IsNotFound(err) {
			return err
		}
		exists = false
	}

	if !exists {
		return m.Client.Server.Create().SetHostname(hostname).SetArch(runtime.GOARCH).SetOs(runtime.GOOS).SetVersion(version).SetChannel(channel).Exec(context.Background())
	}
	return m.Client.Server.Update().SetHostname(hostname).SetArch(runtime.GOARCH).SetOs(runtime.GOOS).SetVersion(version).SetChannel(channel).Where(server.ID(s.ID)).Exec(context.Background())
}

func (m *Model) UpdateServerStatus(version string, channel server.Channel, status server.UpdateStatus, message string, when time.Time) error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	// Fix #1 hostname must not contain dots and domain
	hostnameParts := strings.Split(hostname, ".")
	hostname = hostnameParts[0]

	s, err := m.Client.Server.Query().Where(server.Hostname(hostname), server.Arch(runtime.GOARCH), server.Os(runtime.GOOS), server.ChannelEQ(channel)).Only(context.Background())
	if err != nil {
		return err
	}

	if message == "" {
		message = s.UpdateMessage
	}

	return m.Client.Server.Update().
		SetVersion(version).
		SetUpdateStatus(status).
		SetUpdateMessage(message).
		SetUpdateWhen(when).
		Where(server.ID(s.ID)).
		Exec(context.Background())
}

func (m *Model) GetServerStatus() (*openuem_ent.Server, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	// Fix #1 hostname must not contain dots and domain
	hostnameParts := strings.Split(hostname, ".")
	hostname = hostnameParts[0]

	server, err := m.Client.Server.Query().Where(server.Hostname(hostname)).Only(context.Background())
	if err != nil {
		return nil, err
	}

	return server, nil
}
