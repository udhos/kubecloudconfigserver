package refresh

import (
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestCloseRefresh(t *testing.T) {

	debug := true

	r := New("amqp://guest:guest@rabbitmq:5672/", "app", []string{"app"}, debug, nil)

	wait := time.Second
	t.Logf("giving a time before interrupting the refresh goroutine: %v", wait)
	time.Sleep(wait)

	t.Logf("closing refresh")
	r.Close()

	begin := time.Now()

	timeout := 20 * time.Second

	for !r.hasExited() {
		elap := time.Since(begin)
		if elap > timeout {
			t.Errorf("refresh goroutine has never exited: timeout=%v", timeout)
			return
		}
		sleep := 2 * time.Second
		t.Logf("refresh goroutine has not exited elap=%v timeout=%v, sleeping for %v", elap, timeout, sleep)
		time.Sleep(sleep)
	}
}

func TestCloseDelivery(t *testing.T) {

	var lock sync.Mutex
	var closed bool
	var exited bool

	ch := make(chan string)
	done := make(chan struct{})

	isClosed := func() bool {
		lock.Lock()
		defer lock.Unlock()
		return closed
	}

	doClose := func() {
		lock.Lock()
		defer lock.Unlock()
		close(done)
		closed = true
	}

	hasExited := func() bool {
		lock.Lock()
		defer lock.Unlock()
		return exited
	}

	exit := func() {
		lock.Lock()
		defer lock.Unlock()
		exited = true
	}

	body := []byte(`{"type":"RefreshRemoteApplicationEvent","destinationService":"app:"}`)

	debug := true

	go func() {
		t.Logf("delivery goroutine started")
		handleDelivery(isClosed, body, []string{"app"}, ch, done, debug)
		exit()
	}()

	wait := time.Second
	t.Logf("giving a time before interrupting delivery goroutine: %v", wait)
	time.Sleep(wait)

	t.Logf("interrupting delivery goroutine")
	doClose()

	begin := time.Now()

	timeout := 20 * time.Second

	for !hasExited() {
		if time.Since(begin) > timeout {
			t.Errorf("delivery goroutine has never exited")
			return
		}
		sleep := 2 * time.Second
		t.Logf("delivery goroutine has not exited, sleeping for %v", sleep)
		time.Sleep(sleep)
	}
}

// go test -v ./refresh -run=TestCloseConsume
func TestCloseConsume(t *testing.T) {

	debug := true

	r := New("amqp://guest:guest@rabbitmq:5672/", "app", []string{"app"}, debug, &amqpMock{})

	if r == nil {
		t.Errorf("ugh")
	}

	wait := time.Second
	t.Logf("giving a time before interrupting the refresh goroutine: %v", wait)
	time.Sleep(wait)

	t.Logf("closing refresh")
	r.Close()

	begin := time.Now()

	timeout := 20 * time.Second

	for !r.hasExited() {
		elap := time.Since(begin)
		if elap > timeout {
			t.Errorf("refresh goroutine has never exited: timeout=%v", timeout)
			return
		}
		sleep := 2 * time.Second
		t.Logf("refresh goroutine has not exited elap=%v timeout=%v, sleeping for %v", elap, timeout, sleep)
		time.Sleep(sleep)
	}
}

// go test -v ./refresh -run=TestCloseConsumeWithChannel
func TestCloseConsumeWithChannel(t *testing.T) {

	debug := true

	r := New("amqp://guest:guest@rabbitmq:5672/", "app", []string{"app"}, debug, &amqpMock{})

	if r == nil {
		t.Errorf("ugh")
	}

	wait := time.Second
	t.Logf("giving a time before interrupting the refresh goroutine: %v", wait)
	time.Sleep(wait)

	t.Logf("closing refresh")
	r.Close()

	timeout := time.NewTimer(20 * time.Second)
LOOP:
	for {
		select {
		case _, ok := <-r.C:
			if !ok {
				t.Logf("refresh gorouting exited!")
				break LOOP
			}
		case <-timeout.C:
			t.Errorf("refresh goroutine has never exited: timeout=%v", timeout)
			break LOOP
		}
	}
}

type amqpMock struct {
}

func (a *amqpMock) dial(isClosed func() bool, amqpURL string, sleep, timeout time.Duration) *amqp.Connection {
	return &amqp.Connection{}
}

func (a *amqpMock) closeConn(conn *amqp.Connection) error {
	return nil
}

func (a *amqpMock) channel(conn *amqp.Connection) (*amqp.Channel, error) {
	return &amqp.Channel{}, nil
}

func (a *amqpMock) closeChannel(ch *amqp.Channel) error {
	return nil
}
func (a *amqpMock) cancel(ch *amqp.Channel, consumerTag string) error {
	return nil
}

func (a *amqpMock) exchangeDeclare(ch *amqp.Channel, exchangeName, exchangeType string) error {
	return nil
}

func (a *amqpMock) queueDeclare(ch *amqp.Channel, queueName string) (amqp.Queue, error) {
	return amqp.Queue{}, nil
}

func (a *amqpMock) queueBind(ch *amqp.Channel, queueName, routingKey, exchangeName string) error {
	return nil
}

func (a *amqpMock) consume(ch *amqp.Channel, queueName, consumerTag string) (<-chan amqp.Delivery, error) {
	return make(<-chan amqp.Delivery), nil // reading this will block forever
}
