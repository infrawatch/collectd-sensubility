package sensu

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/paramite/collectd-sensubility/config"
	"github.com/streadway/amqp"
)

const (
	KEEPALIVE_QUEUE = "keepalives"
	RESULTS_QUEUE   = "results"
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

type SensuKeepalive struct {
	Name         string   `json:"name"`
	Address      string   `json:"address"`
	Subscription []string `json:"subscriptions"`
	Version      string   `json:"version"`
	Timestamp    int64    `json:"timestamp"`
}

type SensuConnector struct {
	Address           string
	Subscription      []string
	ClientName        string
	ClientAddress     string
	KeepaliveInterval int
	queueName         string
	exchangeName      string
	inConnection      *amqp.Connection
	outConnection     *amqp.Connection
	inChannel         *amqp.Channel
	outChannel        *amqp.Channel
	queue             amqp.Queue
	consumer          <-chan amqp.Delivery
}

func NewSensuConnector(cfg *config.Config) (*SensuConnector, error) {
	var connector SensuConnector
	connector.Address = cfg.Sections["sensu"].Options["connection"].GetString()
	connector.Subscription = cfg.Sections["sensu"].Options["subscriptions"].GetStrings(",")
	connector.ClientName = cfg.Sections["sensu"].Options["client_name"].GetString()
	connector.ClientAddress = cfg.Sections["sensu"].Options["client_address"].GetString()
	connector.KeepaliveInterval = cfg.Sections["sensu"].Options["keepalive_interval"].GetInt()

	connector.exchangeName = fmt.Sprintf("client:%s", connector.ClientName)
	connector.queueName = fmt.Sprintf("%s-collectd-%d", connector.ClientName, time.Now().Unix())

	err := connector.Connect()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &connector, nil
}

func (self *SensuConnector) Connect() error {
	var err error
	self.inConnection, err = amqp.Dial(self.Address)
	if err != nil {
		return errors.Trace(err)
	}

	self.outConnection, err = amqp.Dial(self.Address)
	if err != nil {
		return errors.Trace(err)
	}

	self.inChannel, err = self.inConnection.Channel()
	if err != nil {
		return errors.Trace(err)
	}

	self.outChannel, err = self.outConnection.Channel()
	if err != nil {
		return errors.Trace(err)
	}

	// declare an exchange for this client
	err = self.inChannel.ExchangeDeclare(
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
	self.queue, err = self.inChannel.QueueDeclare(
		self.queueName, // name
		false,          // durable
		false,          // delete unused
		false,          // exclusive
		false,          // no-wait
		nil,            // arguments
	)
	if err != nil {
		return errors.Trace(err)
	}

	// register consumer
	self.consumer, err = self.inChannel.Consume(
		self.queue.Name, // queue
		self.ClientName, // consumer
		false,           // auto ack
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
		err := self.inChannel.QueueBind(
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

func (self *SensuConnector) ReConnect() error {

	return nil
}

func (self *SensuConnector) Disconnect() {
	self.inChannel.Close()
	self.outChannel.Close()
	self.inConnection.Close()
	self.outConnection.Close()
}

func (self *SensuConnector) Start(outchan chan interface{}, inchan chan interface{}) {
	// receiving loop
	go func() {
		for req := range self.consumer {
			var request SensuCheckRequest
			err := json.Unmarshal(req.Body, &request)
			req.Ack(false)
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
				if err != nil {
					//TODO: log warning
					continue
				}
				err = self.outChannel.Publish(
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
				}
			default:
				//TODO: log warning
			}
		}
	}()

	// keepalive loop
	go func() {
		for {
			body, err := json.Marshal(SensuKeepalive{
				Name:         self.ClientName,
				Address:      self.ClientAddress,
				Subscription: self.Subscription,
				Version:      "collectd",
				Timestamp:    time.Now().Unix(),
			})
			if err != nil {
				//TODO: log warning
				continue
			}
			err = self.outChannel.Publish(
				"",
				KEEPALIVE_QUEUE, // routing to 0 or more queues
				false,           // mandatory
				false,           // immediate
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
			}
			time.Sleep(time.Duration(self.KeepaliveInterval) * time.Second)
		}
	}()
}
