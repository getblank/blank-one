package main

import (
	"flag"
	"fmt"

	"github.com/getblank/blank-one/internet"
	"github.com/getblank/blank-one/intranet"
	"github.com/getblank/blank-one/logging"
	_ "github.com/getblank/blank-one/scheduler"
	"github.com/getblank/blank-sr/config"
)

var (
	buildTime string
	gitHash   string
	version   = "0.0.43"
)

var log = logging.Logger()

func main() {
	showVer := flag.Bool("v", false, "print version and exit")
	flag.Parse()
	if *showVer {
		printVersion()
		return
	}

	defer func() {
		if err := log.Sync(); err != nil {
			panic(err)
		}
	}()

	config.Init("./config.json")
	go internet.Init(version)
	intranet.Init()
}

func printVersion() {
	fmt.Printf("blank-one: \tv%s \t build time: %s \t commit hash: %s \n", version, buildTime, gitHash)
}
