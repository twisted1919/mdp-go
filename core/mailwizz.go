package core

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"
)

type mailwizzCampaign struct {
	sync.WaitGroup
	Type      string
	Processes int
	Limit     int
	Offset    int
	Pause     time.Duration
}

func (c *mailwizzCampaign) work(mwz mailwizz) {
	pm(0, fmt.Sprintf("New batch of %d sets of %s campaigns.", c.Processes, c.Type))
	c.Add(c.Processes)
	for i := 0; i < c.Processes; i++ {
		if c.Pause > 0 {
			time.Sleep(time.Second * c.Pause)
		}
		go func(c *mailwizzCampaign) {
			defer c.Done()
			cmd := exec.Command(
				mwz.PhpCliBinaryPath,
				mwz.ConsolePath,
				mwz.CommandName,
				fmt.Sprintf("--campaigns_offset=%d", c.Offset),
				fmt.Sprintf("--campaigns_limit=%d", c.Limit),
				fmt.Sprintf("--campaigns_type=%s", c.Type),
			)
			err := cmd.Start()
			if err != nil {
				log.Fatal(err)
			}
			err = cmd.Wait()
			if err != nil {
				pm(0, fmt.Sprintf("Exec error: %s", err))
			}
		}(c)
	}
	c.Wait()
}

type mailwizz struct {
	PhpCliBinaryPath string
	ConsolePath      string
	CommandName      string
	Campaigns        []mailwizzCampaign
}

func initSendCampaignsCommands() {
	if len(config.Mailwizz) == 0 {
		return
	}
	for _, mwz := range config.Mailwizz {
		if len(mwz.PhpCliBinaryPath) == 0 || len(mwz.ConsolePath) == 0 {
			continue
		}
		_, err := os.Stat(mwz.PhpCliBinaryPath)
		if err != nil {
			log.Fatalf("PHP binary error: %s", err)
		}
		_, err = os.Stat(mwz.ConsolePath)
		if err != nil {
			log.Fatalf("MAILWIZZ APP console error: %s", err)
		}
		for _, mcmp := range mwz.Campaigns {
			if mcmp.Processes < 1 {
				continue
			}
			go func(mcmp mailwizzCampaign, mwz mailwizz) {
				for {
					mcmp.work(mwz)
				}
			}(mcmp, mwz)
		}
	}
}
