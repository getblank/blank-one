package main

import (
	"flag"
	"fmt"

	log "github.com/Sirupsen/logrus"

	"github.com/getblank/blank-one/internet"
	"github.com/getblank/blank-one/intranet"
)

var (
	buildTime string
	gitHash   string
	version   = "0.0.0"
)

func main() {
	// log.SetFormatter(&log.JSONFormatter{})
	log.SetFormatter(&log.TextFormatter{})

	showVer := flag.Bool("v", false, "print version and exit")
	if *showVer {
		printVersion()
		return
	}

	go internet.Init(version)
	intranet.Init()
}

func printVersion() {
	fmt.Printf("blank-one: \tv%s \t build time: %s \t commit hash: %s \n", version, buildTime, gitHash)
}
