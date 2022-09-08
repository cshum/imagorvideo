package main

import (
	"github.com/cshum/imagor/config"
	"github.com/cshum/imagor/config/awsconfig"
	"github.com/cshum/imagor/config/gcloudconfig"
	"github.com/cshum/imagor/config/vipsconfig"
	"github.com/cshum/imagorvideo"
	"os"
)

func main() {
	var server = config.CreateServer(
		os.Args[1:],
		imagorvideo.Config,
		vipsconfig.WithVips,
		awsconfig.WithAWS,
		gcloudconfig.WithGCloud,
	)
	if server != nil {
		server.Run()
	}
}
