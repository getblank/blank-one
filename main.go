package main

import (
	"flag"
	"fmt"
	"os"

	log "github.com/Sirupsen/logrus"

	"github.com/getblank/blank-one/internet"
	"github.com/getblank/blank-one/intranet"
	"github.com/getblank/blank-sr/config"
)

var (
	buildTime string
	gitHash   string
	version   = "0.0.1"
)

func main() {
	// log.SetFormatter(&log.JSONFormatter{})
	log.SetFormatter(&log.TextFormatter{})
	if d := os.Getenv("BLANK_DEBUG"); len(d) > 0 {
		log.SetLevel(log.DebugLevel)
	}

	showVer := flag.Bool("v", false, "print version and exit")
	if *showVer {
		printVersion()
		return
	}

	config.Init("./config.json")
	go internet.Init(version)
	intranet.Init()
}

func printVersion() {
	fmt.Printf("blank-one: \tv%s \t build time: %s \t commit hash: %s \n", version, buildTime, gitHash)
}
