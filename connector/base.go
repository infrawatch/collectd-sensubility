package connector

type Connector interface {
	Connect() error
	Disconnect() error
	Start(out chan interface{}, in chan interface{})
}
