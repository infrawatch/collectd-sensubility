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

const (
	DEFAULT_HOSTNAME = "khokhot.localhost.localdomain"
	RESULTS_QUEUE    = "results"
)

type SensuCheckRequest struct {
	Command string
	Name    string
	Issued  int
}

type SensuCheckResult struct {
	Command  string  `json:"command"`
	Name     string  `json:"name"`
	Issued   int     `json:"issued"`
	Executed int64   `json:"executed"`
	Duration float32 `json:"duration"`
	Output   string  `json:"output"`
	Status   int     `json:"status"`
}

type SensuResult struct {
	Client string           `json:"client"`
	Check  SensuCheckResult `json:"check"`
}

type SensuConnector struct {
	Address      string
	Subscription []string
	connection   *amqp.Connection
	exchangeName string
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
	self.exchangeName = fmt.Sprintf("client:%s", host)
	err = self.channel.ExchangeDeclare(
		self.exchangeName, // name
		"fanout",          // type
		false,             // durable
		false,             // auto-deleted
		false,             // internal
		false,             // no-wait
		nil,               // arguments
	)
	if err != nil {
		return errors.Trace(err)
	}

	// declare a queue for this client
	timestamp := time.Now().Unix()
	self.queue, err = self.channel.QueueDeclare(
		fmt.Sprintf("%s-collectd-%d", host, timestamp), // name
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

func (self *SensuConnector) Disconnect() error {
	self.channel.Close()
	return self.connection.Close()
}

func (self *SensuConnector) Start(outchan chan interface{}, inchan chan interface{}) {
	// receiving loop
	go func() {
		for req := range self.consumer {
			var request SensuCheckRequest
			err := json.Unmarshal(req.Body, &request)
			if err == nil {
				outchan <- request
			} else {
				//TODO: log warning
			}
		}
	}()

	// sending loop
	go func() {
		for res := range inchan {
			switch result := res.(type) {
			case SensuResult:
				body, err := json.Marshal(result)
				fmt.Printf("%s\n", body)
				if err != nil {
					//TODO: log warning
					continue
				}
				err = self.channel.Publish(
					"",
					RESULTS_QUEUE, // routing to 0 or more queues
					false,         // mandatory
					false,         // immediate
					amqp.Publishing{
						Headers:         amqp.Table{},
						ContentType:     "text/json",
						ContentEncoding: "",
						Body:            body,
						DeliveryMode:    amqp.Transient, // 1=non-persistent, 2=persistent
						Priority:        0,              // 0-9
					})
				if err != nil {
					//TODO: log warning
					fmt.Printf("fuck it: %s\n", err)
				}
			default:
				//TODO: log warning
			}
		}
	}()
}
