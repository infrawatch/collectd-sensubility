package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"

	"github.com/infrawatch/apputils/config"
	"github.com/infrawatch/apputils/connector/amqp10"
	connector "github.com/infrawatch/apputils/connector/sensu"
	"github.com/infrawatch/apputils/logging"
	"github.com/infrawatch/collectd-sensubility/formats"
	"github.com/infrawatch/collectd-sensubility/sensu"
)

//Default values for various functions
const (
	DefaultLogPath    = "/var/log/collectd/sensubility.log"
	DefaultConfigPath = "/etc/collectd-sensubility.conf"
	DefaultHostname   = "localhost.localdomain"
	DefaultIP         = "127.0.0.1"
)

//GetHostname returns value of COLLECTD_HOSTNAME env or if not set FQDN of the host
func GetHostname() string {
	if host := os.Getenv("COLLECTD_HOSTNAME"); host != "" {
		return host
	}
	if host, err := os.Hostname(); err == nil {
		return host
	}
	return DefaultHostname
}

//GetOutboundIP returns IP address of external interface
func GetOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return DefaultIP
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

//GetAgentConfigMetadata returns config metadata sctructure for apputils.config usage
func GetAgentConfigMetadata() map[string][]config.Parameter {
	elements := map[string][]config.Parameter{
		"default": []config.Parameter{
			config.Parameter{
				Name:       "log_file",
				Tag:        "",
				Default:    "",
				Validators: []config.Validator{},
			},
			config.Parameter{
				Name:       "log_level",
				Tag:        "",
				Default:    "INFO",
				Validators: []config.Validator{config.StringOptionsValidatorFactory([]string{"DEBUG", "INFO", "WARNING", "ERROR"})},
			},
			config.Parameter{
				Name:       "allow_exec",
				Tag:        "",
				Default:    "true",
				Validators: []config.Validator{config.BoolValidatorFactory()},
			},
		},
		"sensu": []config.Parameter{
			config.Parameter{
				Name:       "connection",
				Tag:        "",
				Default:    "",
				Validators: []config.Validator{},
			},
			config.Parameter{
				Name:       "subscriptions",
				Tag:        "",
				Default:    "all,default",
				Validators: []config.Validator{},
			},
			config.Parameter{
				Name:       "client_name",
				Tag:        "",
				Default:    GetHostname(),
				Validators: []config.Validator{},
			},
			config.Parameter{
				Name:       "client_address",
				Tag:        "",
				Default:    GetOutboundIP(),
				Validators: []config.Validator{},
			},
			config.Parameter{
				Name:       "keepalive_interval",
				Tag:        "",
				Default:    20,
				Validators: []config.Validator{config.IntValidatorFactory()},
			},
			config.Parameter{
				Name:       "tmp_base_dir",
				Tag:        "",
				Default:    "/var/tmp/collectd-sensubility-checks",
				Validators: []config.Validator{},
			},
			config.Parameter{
				Name:       "shell_path",
				Tag:        "",
				Default:    "/usr/bin/sh",
				Validators: []config.Validator{},
			},
			config.Parameter{
				Name:       "worker_count",
				Tag:        "",
				Default:    2,
				Validators: []config.Validator{config.IntValidatorFactory()},
			},
			config.Parameter{
				Name:       "checks",
				Tag:        "",
				Default:    "{}",
				Validators: []config.Validator{},
			},
		},
		"amqp1": []config.Parameter{
			config.Parameter{
				Name:       "connection",
				Tag:        "",
				Default:    "",
				Validators: []config.Validator{},
			},
			config.Parameter{
				Name:       "client_name",
				Tag:        "",
				Default:    GetHostname(),
				Validators: []config.Validator{},
			},
			config.Parameter{
				Name:       "send_timeout",
				Tag:        "",
				Default:    2,
				Validators: []config.Validator{config.IntValidatorFactory()},
			},
			config.Parameter{
				Name:       "results_channel",
				Tag:        "",
				Default:    "collectd/events",
				Validators: []config.Validator{},
			},
			config.Parameter{
				Name:       "results_format",
				Tag:        "",
				Default:    "smartgateway",
				Validators: []config.Validator{config.StringOptionsValidatorFactory([]string{"smartgateway", "sensu"})},
			},
			config.Parameter{
				Name:       "listen_channels",
				Tag:        "",
				Default:    "",
				Validators: []config.Validator{},
			},
			config.Parameter{
				Name:       "listen_prefetch",
				Tag:        "",
				Default:    -1,
				Validators: []config.Validator{config.IntValidatorFactory()},
			},
		},
	}
	return elements
}

func main() {
	debug := flag.Bool("debug", false, "enables debugging logs")
	verbose := flag.Bool("verbose", false, "enables informational logs")
	silent := flag.Bool("silent", false, "disables all logs except fatal errors")
	logpath := flag.String("log", DefaultLogPath, "path to log file")
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

	// spawn entities
	metadata := GetAgentConfigMetadata()
	cfg := config.NewINIConfig(metadata, log)
	confPath := os.Getenv("COLLECTD_SENSUBILITY_CONFIG")
	if confPath == "" {
		confPath = DefaultConfigPath
	}
	err = cfg.Parse(confPath)
	if err != nil {
		log.Destroy()
		fmt.Printf("Failed to parse config file: %s\n", err.Error())
		os.Exit(2)
	}

	// recreate logger according to values in config file
	logFile, err := cfg.GetOption("default/log_file")
	if err == nil && logFile.GetString() != "" {
		log, err = logging.NewLogger(level, logFile.GetString())
		if err != nil {
			fmt.Printf("Failed to open log file %s.\n", logFile)
			os.Exit(2)
		}
		defer log.Destroy()
	}
	logLevel, err := cfg.GetOption("default/log_level")
	if err == nil && len(logLevel.GetString()) > 0 {
		switch logLevel.GetString() {
		case "DEBUG":
			log.SetLogLevel(logging.DEBUG)
		case "INFO":
			log.SetLogLevel(logging.INFO)
		case "WARNING":
			log.SetLogLevel(logging.WARN)
		case "ERROR":
			log.SetLogLevel(logging.ERROR)
		}
	}

	requests := make(chan interface{})
	sensuResults := make(chan interface{})
	amqpResults := make(chan interface{})
	wait := make(chan bool)
	defer close(sensuResults)
	defer close(amqpResults)

	reportSensu := false
	sensuConnector := &connector.SensuConnector{}
	if sect, ok := cfg.Sections["sensu"]; ok {
		if opt, ok := sect.Options["connection"]; ok {
			if len(opt.GetString()) > 0 {
				sensuConnector, err = connector.ConnectSensu(cfg, log)
				if err != nil {
					log.Metadata(map[string]interface{}{"error": err, "connection": opt.GetString()})
					log.Error("Failed to spawn RabbitMQ connector.")
					os.Exit(2)
				}
				defer sensuConnector.Disconnect()
				sensuConnector.Start(requests, sensuResults)
				reportSensu = true
			}
		}
	}

	reportAmqp := false
	amqpAddr := "collectd/events"
	amqpConnector := &amqp10.AMQP10Connector{}
	if sect, ok := cfg.Sections["amqp1"]; ok {
		if opt, ok := sect.Options["connection"]; ok {
			if len(opt.GetString()) > 0 {
				amqpConnector, err = amqp10.ConnectAMQP10(cfg, log)
				if err != nil {
					log.Metadata(map[string]interface{}{"error": err, "connection": opt.GetString()})
					log.Error("Failed to spawn AMQP1.0 connector.")
					os.Exit(2)
				}
				defer amqpConnector.Disconnect()
				amqpConnector.Start(requests, amqpResults)
				reportAmqp = true

				addrOpt, err := cfg.GetOption("amqp1/results_channel")
				if err != nil || len(addrOpt.GetString()) <= 0 {
					log.Metadata(map[string]interface{}{
						"error":   err,
						"default": amqpAddr,
					})
					log.Info("Failed to get amqp1/results_channel configuration value. Using default value.")
				} else {
					amqpAddr = addrOpt.GetString()
				}
			}
		}
	}

	sensuExecutor, err := sensu.NewExecutor(cfg, log)
	if err != nil {
		log.Metadata(map[string]interface{}{"error": err})
		log.Error("Failed to spawn check executor.")
		os.Exit(2)
	}
	defer sensuExecutor.Clean()

	sensuScheduler, err := sensu.NewScheduler(cfg, log)
	if err != nil {
		log.Metadata(map[string]interface{}{"error": err})
		log.Error("Failed to spawn check scheduler.")
		os.Exit(2)
	}
	sensuScheduler.Start(requests)

	// spawn worker goroutines

	workers := cfg.Sections["sensu"].Options["worker_count"].GetInt()
	for i := int64(0); i < workers; i++ {
		go func(amqpAddr *string, amqpResults chan interface{}) {
			for {
				req := <-requests
				switch req := req.(type) {
				case connector.CheckRequest:
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
					if reportSensu {
						sensuResults <- res
					}
					if reportAmqp {
						var body []byte
						if cfg.Sections["amqp1"].Options["results_format"].GetString() == "sensu" {
							body, err = json.Marshal(res)
						} else {
							sgres, errr := formats.CreateSGResult(res)
							if errr == nil {
								body, err = json.Marshal(sgres)
							} else {
								err = errr
							}
						}
						if err != nil {
							log.Metadata(map[string]interface{}{
								"error":  err,
								"result": res,
							})
							log.Error("Failed to marshal check result.")
							continue
						}
						msg := amqp10.AMQP10Message{
							Address: *amqpAddr,
							Body:    string(body),
						}
						amqpResults <- msg
					}
				default:
					log.Metadata(map[string]interface{}{
						"error":   err,
						"request": req,
					})
					log.Error("Failed to execute requested command.")
				}
			}
		}(&amqpAddr, amqpResults)
	}
	<-wait
}
