package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/paramite/collectd-sensubility/config"
	"github.com/paramite/collectd-sensubility/sensu"
	"github.com/rs/zerolog"
)

const DEFAULT_CONFIG_PATH = "/etc/collectd-sensubility.conf"

func main() {
	debug := flag.Bool("debug", false, "enables debugging logs")
	verbose := flag.Bool("verbose", false, "enables debugging logs")
	logpath := flag.String("log", "/var/log/collectd/sensubility.log", "path to log file")
	flag.Parse()

	// set logging
	logfile, err := os.OpenFile(*logpath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	defer logfile.Close()
	if err != nil {
		fmt.Printf("Failed to open log file %s.\n", *logpath)
		os.Exit(2)
	}
	zerolog.SetGlobalLevel(zerolog.WarnLevel)
	if *verbose {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	} else if *debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	log := zerolog.New(logfile).With().Timestamp().Logger()

	// spawn entities
	metadata := config.GetAgentConfigMetadata()
	cfg, err := config.NewConfig(metadata, log.With().Str("component", "config-parser").Logger())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to parse config file.")
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
	sensuConnector, err := sensu.NewConnector(cfg, log.With().Str("component", "sensu-connector").Logger())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to spawn RabbitMQ connector.")
		os.Exit(2)
	}
	defer sensuConnector.Disconnect()

	sensuScheduler, err := sensu.NewScheduler(cfg, log.With().Str("component", "sensu-scheduler").Logger())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to spawn check scheduler.")
		os.Exit(2)
	}

	sensuExecutor, err := sensu.NewExecutor(cfg, log.With().Str("component", "sensu-executor").Logger())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to spawn check executor.")
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
						log.Error().Err(err).Str("request", reqstr).Msg("Failed to execute requested command.")
						continue
					}
					results <- res
				default:
					log.Error().Err(err).Str("request", fmt.Sprintf("%v", req)).Msg("Failed to execute requested command.")
				}
			}
		}()
	}
	<-wait
}
