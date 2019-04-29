package main

import (
	"fmt"
	"os"
	"time"

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
	fmt.Printf("Test\n")
	sensuConnector, err := sensu.NewSensuConnector(cfg)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(2)
	}
	defer sensuConnector.Disconnect()

	requests := make(chan interface{})
	results := make(chan interface{})
	defer close(results)

	sensuConnector.Start(requests, results)
	for {
		req := <-requests
		switch req := req.(type) {
		case sensu.SensuCheckRequest:
			res := sensu.SensuResult{
				Client: config.GetHostname(),
				Check: sensu.SensuCheckResult{
					Command:  req.Command,
					Name:     req.Name,
					Issued:   req.Issued,
					Executed: time.Now().Unix(),
					Duration: 0.10,
					Output:   "Wooot?\n",
					Status:   0,
				},
			}
			results <- res
		default:
			fmt.Printf("[%T] %v\n", req, req)
		}
	}

	//fmt.Sprintf("%s", request.Command)
	// cmd := exec.Command(request.Command)
	// stdout, err := cmd.StdoutPipe()
	// err = cmd.Start()
	fmt.Printf("End")
}
