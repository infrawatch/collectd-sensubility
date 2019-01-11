package main

import (
	"fmt"
	"os"

	"github.com/paramite/collectd-sensubility/config"
	"github.com/paramite/collectd-sensubility/connector"
)

const DEFAULT_CONFIG_PATH = "/etc/collectd-sensubility.conf"

func main() {
	metadata := config.GetAgentConfigMetadata()
	cfg, err := config.NewConfig(metadata)
	if err != nil {
		panic(err.Error())
	}

	confPath := os.Getenv("COLLECTD_SENSUBILITY_CONFIG")
	if confPath == "" {
		confPath = DEFAULT_CONFIG_PATH
	}
	err = cfg.Parse(confPath)
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("Test\n")
	sensu, err := connector.NewSensuConnector(cfg)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(2)
	}
	defer sensu.Disconnect()
	channel := make(chan interface{})
	go sensu.Process(channel)
	for {
		msg := <-channel
		switch msg := msg.(type) {
		case string:
			fmt.Printf("command: %q\n", msg)
		default:
			fmt.Printf("[%T] %v\n", msg, msg)
		}
	}
	fmt.Printf("End")
}
