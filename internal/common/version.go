package common

import (
	"github.com/doncicuto/openuem_ent/server"
)

func (us *UpdaterService) SetServer() error {
	return us.Model.SetServer(us.Version, server.Channel(us.Channel))
}
