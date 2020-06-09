package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"

	"github.com/infrawatch/apputils/config"
	"github.com/infrawatch/apputils/connector"
	"github.com/infrawatch/apputils/logging"

	"github.com/infrawatch/collectd-sensubility/sensu"
)

//Default values for various functions
const (
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
				Default:    "/var/log/collectd-sensubility.log",
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
				Default:    5666,
				Validators: []config.Validator{config.IntValidatorFactory()},
			},
			config.Parameter{
				Name:       "results_address",
				Tag:        "",
				Default:    "collectd/checks",
				Validators: []config.Validator{},
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
	metadata := GetAgentConfigMetadata()
	cfg := config.NewINIConfig(metadata, log)
	if err != nil {
		log.Metadata(map[string]interface{}{"error": err})
		log.Error("Failed to parse config file.")
		os.Exit(2)
	}
	confPath := os.Getenv("COLLECTD_SENSUBILITY_CONFIG")
	if confPath == "" {
		confPath = DefaultConfigPath
	}
	err = cfg.Parse(confPath)
	if err != nil {
		fmt.Printf("Failed to parse log file: %s\n", err.Error())
		os.Exit(2)
	}

	requests := make(chan interface{})
	SensuResults := make(chan interface{})
	AmqpResults := make(chan interface{})
	wait := make(chan bool)
	defer close(SensuResults)
	defer close(AmqpResults)

	reportSensu := false
	sensuConnector := &connector.SensuConnector{}
	if sect, ok := cfg.Sections["sensu"]; ok {
		if opt, ok := sect.Options["connection"]; ok {
			if len(opt.GetString()) > 0 {
				sensuConnector, err = connector.NewSensuConnector(cfg, log)
				if err != nil {
					log.Metadata(map[string]interface{}{"error": err, "connection": opt.GetString()})
					log.Error("Failed to spawn RabbitMQ connector.")
					os.Exit(2)
				}
				defer sensuConnector.Disconnect()
				sensuConnector.Start(requests, SensuResults)
				reportSensu = true
			}
		}
	}

	reportAmqp := false
	amqpAddr := "collectd/checks"
	amqpConnector := &connector.AMQP10Connector{}
	if sect, ok := cfg.Sections["amqp1"]; ok {
		if opt, ok := sect.Options["connection"]; ok {
			if len(opt.GetString()) > 0 {
				amqpConnector, err = connector.NewAMQP10Connector(cfg, log)
				if err != nil {
					log.Metadata(map[string]interface{}{"error": err, "connection": opt.GetString()})
					log.Error("Failed to spawn AMQP1.0 connector.")
					os.Exit(2)
				}
				err = amqpConnector.Connect()
				if err != nil {
					log.Metadata(map[string]interface{}{"error": err, "connection": opt.GetString()})
					log.Error("Failed to connect to AMQP1.0 message bus.")
					os.Exit(2)
				}
				defer amqpConnector.Disconnect()
				amqpConnector.Start(requests, AmqpResults)
				reportAmqp = true

				addrOpt, err := cfg.GetOption("amqp1/results_address")
				if err != nil || len(addrOpt.GetString()) <= 0 {
					log.Metadata(map[string]interface{}{
						"error":   err,
						"default": amqpAddr,
					})
					log.Info("Failed to get amqp1/results_address configuration value. Using default value.")
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
		go func(amqpAddr *string) {
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
						SensuResults <- res
					}
					if reportAmqp {
						//TODO: Format message struct from the CheckResult and send it to AmqpResults
						body, err := json.Marshal(res)
						if err != nil {
							log.Metadata(map[string]interface{}{
								"error":  err,
								"result": res,
							})
							log.Error("Failed to marshal check result.")
							continue
						}
						msg := connector.AMQP10Message{
							Address: *amqpAddr,
							Body:    string(body),
						}
						AmqpResults <- msg
					}
				default:
					log.Metadata(map[string]interface{}{
						"error":   err,
						"request": req,
					})
					log.Error("Failed to execute requested command.")
				}
			}
		}(&amqpAddr)
	}
	<-wait
}
