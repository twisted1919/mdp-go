package core

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

var (
	config *configParams
)

type configParams struct {
	Debug           *debugParams
	DirectoryPickup *directoryPickup
	DeliveryServers []deliveryServer
	Mailwizz        []mailwizz
}

func loadConfigFromFile() {

	currentPath, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}
	configFilePath := currentPath + string(os.PathSeparator) + "config.json"

	_, err = os.Stat(configFilePath)
	if err != nil {
		log.Fatalf("Configuration file stat error: %s", err)
	}

	fileBytes, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		log.Fatalf("Configuration file read error: %s", err)
	}

	config = &configParams{}
	err = json.Unmarshal(fileBytes, &config)
	if err != nil {
		log.Fatalf("Configuration file marshal error: %s", err)
	}
	fileBytes = nil

	if len(config.DeliveryServers) == 0 {
		log.Fatal("Cannot proceed without delivery servers, please add them into the configuration file!")
	}

	if len(config.DirectoryPickup.StorageDirectory) == 0 {
		log.Fatalf("Please provide directory pickup path in your configuration file located at %s!", configFilePath)
	}

	dinfo, err := os.Stat(config.DirectoryPickup.StorageDirectory)
	if err != nil {
		log.Fatalf("Directory pickup stat error: %s", err)
	}

	if !dinfo.IsDir() {
		log.Fatalf("%s is not a directory!", config.DirectoryPickup.StorageDirectory)
	}
}
