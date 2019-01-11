package connector

type Connector interface {
	Connect() error
	Disconnect()
	Process(channel chan interface{})
}
