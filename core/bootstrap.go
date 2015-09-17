package core

import (
	"time"
)

// Bootstrap the daemon
func Bootstrap() {
	loadConfigFromFile()
	deliveryServersConnectionTests()
	config.DirectoryPickup.start()
	initSendCampaignsCommands()

	ticker := time.NewTicker(time.Second * 60)
	for _ = range ticker.C {
	}
}
