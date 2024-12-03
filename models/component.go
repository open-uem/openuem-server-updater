package models

import (
	"context"
	"os"
	"runtime"

	"github.com/doncicuto/openuem_ent/component"
)

func (m *Model) UpdateComponent(c component.Component, version string, channel component.Channel, status component.UpdateStatus, message string) error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	s, err := m.Client.Component.Query().Where(component.Hostname(hostname), component.ComponentEQ(c), component.Arch(runtime.GOARCH), component.Os(runtime.GOOS), component.Version(version), component.ChannelEQ(channel)).Only(context.Background())
	if err != nil {
		return err
	}

	return m.Client.Component.Update().
		SetVersion(version).
		SetUpdateStatus(status).
		SetUpdateMessage(message).
		Where(component.ID(s.ID)).
		Exec(context.Background())
}

func (m *Model) RollbackComponent(c component.Component, version string, channel component.Channel, status component.UpdateStatus, message string) error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	s, err := m.Client.Component.Query().Where(component.Hostname(hostname), component.ComponentEQ(c), component.Arch(runtime.GOARCH), component.Os(runtime.GOOS), component.Version(version), component.ChannelEQ(channel)).Only(context.Background())
	if err != nil {
		return err
	}

	return m.Client.Component.Update().
		SetUpdateStatus(status).
		SetUpdateMessage(message).
		Where(component.ID(s.ID)).
		Exec(context.Background())
}
