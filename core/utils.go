package core

import (
	"fmt"
)

type debugParams struct {
	Enabled bool
	Level   int
}

func (d *debugParams) printMsg(level int, message string) {
	if !d.Enabled || level > d.Level {
		return
	}
	fmt.Println(message)
}

func pm(level int, message string) {
	config.Debug.printMsg(level, message)
}
