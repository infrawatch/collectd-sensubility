package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/infrawatch/collectd-sensubility/config"
	"github.com/infrawatch/collectd-sensubility/logging"
	"github.com/infrawatch/collectd-sensubility/sensu"
)

const DEFAULT_CONFIG_PATH = "/etc/collectd-sensubility.conf"

func main() {
	debug := flag.Bool("debug", false, "enables debugging logs")
	verbose := flag.Bool("verbose", false, "enables informational logs")
	silent := flag.Bool("silent", false, "disables all logs except fatal errors")
	logpath := flag.String("log", "/var/log/collectd/sensubility.log", "path to log file")
	flag.Parse()

	// set logging
	level := logging.WARN
	if *verbose {
		level = logging.INFO
	} else if *debug {
		level = logging.DEBUG
	} else if *silent {
		level = logging.ERROR
	}
	log, err := logging.NewLogger(level, *logpath)
	if err != nil {
		fmt.Printf("Failed to open log file %s.\n", *logpath)
		os.Exit(2)
	}
	defer log.Destroy()

	// spawn entities
	metadata := config.GetAgentConfigMetadata()
	cfg, err := config.NewConfig(metadata, log)
	if err != nil {
		log.Metadata(map[string]interface{}{"error": err})
		log.Error("Failed to parse config file.")
		os.Exit(2)
	}
	confPath := os.Getenv("COLLECTD_SENSUBILITY_CONFIG")
	if confPath == "" {
		confPath = DEFAULT_CONFIG_PATH
	}
	err = cfg.Parse(confPath)
	if err != nil {
		panic(err.Error())
	}
	sensuConnector, err := sensu.NewConnector(cfg, log)
	if err != nil {
		log.Metadata(map[string]interface{}{"error": err})
		log.Error("Failed to spawn RabbitMQ connector.")
		os.Exit(2)
	}
	defer sensuConnector.Disconnect()

	sensuScheduler, err := sensu.NewScheduler(cfg, log)
	if err != nil {
		log.Metadata(map[string]interface{}{"error": err})
		log.Error("Failed to spawn check scheduler.")
		os.Exit(2)
	}

	sensuExecutor, err := sensu.NewExecutor(cfg, log)
	if err != nil {
		log.Metadata(map[string]interface{}{"error": err})
		log.Error("Failed to spawn check executor.")
		os.Exit(2)
	}
	defer sensuExecutor.Clean()

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
						reqstr := fmt.Sprintf("Request{name=%s, command=%s, issued=%d}", req.Name, req.Command, req.Issued)
						log.Metadata(map[string]interface{}{
							"error":   err,
							"request": reqstr,
						})
						log.Error("Failed to execute requested command.")
						continue
					}
					results <- res
				default:
					log.Metadata(map[string]interface{}{
						"error":   err,
						"request": req,
					})
					log.Error("Failed to execute requested command.")
				}
			}
		}()
	}
	<-wait
}
