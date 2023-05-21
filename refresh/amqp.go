// Package refresh reads config refresh notification from AMQP.
package refresh

import (
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type amqpEngine interface {
	dial(isClosed func() bool, amqpURL string, sleep, timeout time.Duration) *amqp.Connection
	closeConn(conn *amqp.Connection) error
	channel(conn *amqp.Connection) (*amqp.Channel, error)
	closeChannel(ch *amqp.Channel) error
	cancel(ch *amqp.Channel, consumerTag string) error
	exchangeDeclare(ch *amqp.Channel, exchangeName, exchangeType string) error
	queueDeclare(ch *amqp.Channel, queueName string) (amqp.Queue, error)
	queueBind(ch *amqp.Channel, queueName, routingKey, exchangeName string) error
	consume(ch *amqp.Channel, queueName, consumerTag string) (<-chan amqp.Delivery, error)
}

type amqpReal struct{}

func (a *amqpReal) dial(isClosed func() bool, amqpURL string, sleep, timeout time.Duration) *amqp.Connection {
	return dial(isClosed, amqpURL, sleep, timeout)
}

func (a *amqpReal) closeConn(conn *amqp.Connection) error {
	return conn.Close()
}

func (a *amqpReal) channel(conn *amqp.Connection) (*amqp.Channel, error) {
	ch, err := conn.Channel()
	return ch, err
}

func (a *amqpReal) closeChannel(ch *amqp.Channel) error {
	return ch.Close()
}

func (a *amqpReal) cancel(ch *amqp.Channel, consumerTag string) error {
	return ch.Cancel(consumerTag, false)
}

func (a *amqpReal) exchangeDeclare(ch *amqp.Channel, exchangeName, exchangeType string) error {
	err := ch.ExchangeDeclare(
		exchangeName, // name of the exchange
		exchangeType, // type
		true,         // durable
		false,        // delete when complete
		false,        // internal
		false,        // noWait
		nil,          // arguments
	)
	return err
}

func (a *amqpReal) queueDeclare(ch *amqp.Channel, queueName string) (amqp.Queue, error) {
	q, err := ch.QueueDeclare(
		queueName, // name
		false,     // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	return q, err
}

func (a *amqpReal) queueBind(ch *amqp.Channel, queueName, routingKey, exchangeName string) error {
	err := ch.QueueBind(
		queueName,    // name of the queue
		routingKey,   // bindingKey
		exchangeName, // sourceExchange
		false,        // noWait
		nil,          // arguments
	)
	return err
}

func (a *amqpReal) consume(ch *amqp.Channel, queueName, consumerTag string) (<-chan amqp.Delivery, error) {
	msgs, err := ch.Consume(
		queueName,   // queue
		consumerTag, // consumer
		true,        // auto-ack
		false,       // exclusive
		false,       // no-local
		false,       // no-wait
		nil,         // args
	)
	return msgs, err
}

// dial tries forever to connect to amqp URL.
// it always return a valid non-nil connection, unless interrupted by Close().
func dial(isClosed func() bool, amqpURL string, sleep, timeout time.Duration) *amqp.Connection {
	var c *amqp.Connection
	for retry := 0; !isClosed(); retry++ {
		log.Printf("attempt=%d: dialing %s", retry, amqpURL)
		conn, err := amqpDial(amqpURL, timeout)
		if err != nil {
			log.Printf("attempt=%d: dialing %s: error: %v", retry, amqpURL, err)
			time.Sleep(sleep)
			continue
		}
		log.Printf("attempt=%d: dialing %s: connected", retry, amqpURL)
		c = conn
		break
	}
	return c
}

// amqp.Dial but with timeout
func amqpDial(amqpURL string, timeout time.Duration) (*amqp.Connection, error) {
	//conn, err := amqp.Dial(amqpURL)
	config := amqp.Config{
		Heartbeat: 10 * time.Second,
		Locale:    "en_US",
		Dial:      amqp.DefaultDial(timeout),
	}
	conn, err := amqp.DialConfig(amqpURL, config)
	return conn, err
}
