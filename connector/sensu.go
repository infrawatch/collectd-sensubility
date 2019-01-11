package connector

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/paramite/collectd-sensubility/config"
	"github.com/streadway/amqp"
)

const DEFAULT_HOSTNAME = "localhost.localdomain"

type SensuConnector struct {
	Address      string
	Subscription []string
	connection   *amqp.Connection
	channel      *amqp.Channel
	queue        amqp.Queue
	consumer     <-chan amqp.Delivery
}

func NewSensuConnector(cfg *config.Config) (*SensuConnector, error) {
	var connector SensuConnector
	connector.Address = cfg.Sections["sensu"].Options["connection"].GetString()
	connector.Subscription = cfg.Sections["sensu"].Options["subscriptions"].GetStrings(",")
	err := connector.Connect()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &connector, nil
}

func (self *SensuConnector) Connect() error {
	var err error
	self.connection, err = amqp.Dial(self.Address)
	if err != nil {
		return errors.Trace(err)
	}

	self.channel, err = self.connection.Channel()
	if err != nil {
		return errors.Trace(err)
	}

	host := os.Getenv("COLLECTD_HOSTNAME")
	if host == "" {
		host = DEFAULT_HOSTNAME
	}

	// declare an exchange for this client
	err = self.channel.ExchangeDeclare(
		fmt.Sprintf("client:%s", host), // name
		"fanout",                       // type
		false,                          // durable
		false,                          // auto-deleted
		false,                          // internal
		false,                          // no-wait
		nil,                            // arguments
	)
	if err != nil {
		return errors.Trace(err)
	}

	// declare a queue for this client
	timestamp := time.Now().Unix()
	self.queue, err = self.channel.QueueDeclare(
		fmt.Sprintf("%s-collectd-%s", host, timestamp), // name
		false, // durable
		false, // delete unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return errors.Trace(err)
	}

	// register consumer
	self.consumer, err = self.channel.Consume(
		self.queue.Name, // queue
		"collectd",      // consumer
		true,            // auto ack
		false,           // exclusive
		false,           // no local
		false,           // no wait
		nil,             // args
	)
	if err != nil {
		return errors.Trace(err)
	}

	// bind client queue with subscriptions
	for _, sub := range self.Subscription {
		err := self.channel.QueueBind(
			self.queue.Name, // queue name
			"",              // routing key
			sub,             // exchange
			false,
			nil,
		)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (self *SensuConnector) Disconnect() {
	self.channel.Close()
	self.connection.Close()
}

func (self *SensuConnector) Process(channel chan interface{}) {
	var request struct {
		Command string
		Name    string
		Issued  int
	}
	for req := range self.consumer {
		fmt.Sprintf("%s", req.Body)
		err := json.Unmarshal(req.Body, &request)
		if err == nil {
			channel <- request.Command
		} else {
			//TO-DO: log waning
		}
		//fmt.Sprintf("%s", request.Command)
		// cmd := exec.Command(request.Command)
		// stdout, err := cmd.StdoutPipe()
		// err = cmd.Start()
	}
}
