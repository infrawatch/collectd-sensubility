package main

import (
	"fmt"
	"os"

	"github.com/paramite/collectd-sensubility/config"
	"github.com/paramite/collectd-sensubility/sensu"
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
	sensuConnector, err := sensu.NewConnector(cfg)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(2)
	}
	defer sensuConnector.Disconnect()

	sensuScheduler, err := sensu.NewScheduler(cfg)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(2)
	}

	sensuExecutor, err := sensu.NewExecutor(cfg)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(2)
	}

	requests := make(chan interface{})
	results := make(chan interface{})
	wait := make(chan bool)
	defer close(results)

	sensuConnector.Start(requests, results)
	sensuScheduler.Start(requests)
	// spawn worker goroutines
	workers := cfg.Sections["sensu"].Options["worker_count"].GetInt()
	for i := 0; i < workers; i++ {
		go func() {
			for {
				req := <-requests
				switch req := req.(type) {
				case sensu.CheckRequest:
					res, err := sensuExecutor.Execute(req)
					if err != nil {
						//TODO: log warning
						continue
					}
					results <- res
				default:
					//TODO: log warning
				}
			}
		}()
	}
	<-wait
}
