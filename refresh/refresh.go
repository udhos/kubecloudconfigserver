package refresh

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/segmentio/ksuid"
)

// Refresh holds data for worker that reads events from amqp and delivers applications found in channel `Refresh.C`.
type Refresh struct {
	amqpURL           string
	applications      []string
	consumerTag       string
	debug             bool
	dialTimeout       time.Duration
	dialRetryInterval time.Duration
	C                 chan string // Channel C delivers applications names found in events.
	done              chan struct{}
	amqpClient        amqpEngine

	lock   sync.Mutex
	closed bool
	exited bool
}

// New spawns a Refresh worker that reads events from amqp and delivers applications found in channel `Refresh.C`.
func New(amqpURL, consumerTag string, applications []string, debug bool, engine amqpEngine) *Refresh {
	if len(applications) == 0 {
		log.Panicln("refresh.New: slice applications must be non-empty")
	}
	r := &Refresh{
		amqpURL:           amqpURL,
		applications:      applications,
		debug:             debug,
		C:                 make(chan string),
		dialTimeout:       10 * time.Second,
		dialRetryInterval: 5 * time.Second,
		consumerTag:       consumerTag,
		done:              make(chan struct{}),
		amqpClient:        engine,
	}

	if r.amqpClient == nil {
		r.amqpClient = &amqpReal{}
	}

	go serve(r)
	return r
}

// Close interrupts the refresh goroutine to release resources.
func (r *Refresh) Close() {
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.closed {
		return
	}
	close(r.done)
	r.closed = true
}

func (r *Refresh) isClosed() bool {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.closed
}

func serve(r *Refresh) {
	// serve forever, unless interrupted by Close()
	for i := 1; !r.isClosed(); i++ {
		begin := time.Now()
		serveOnce(r, i, begin)
		log.Printf("serve connCount=%d uptime=%v: will restart amqp connection", i, time.Since(begin))
	}
	if r.debug {
		log.Print("refresh.serve: refresh closed, exiting...")
	}

	close(r.C) // close delivery channel to notify readers we finished

	r.lock.Lock()
	r.exited = true
	r.lock.Unlock()
	if r.debug {
		log.Print("refresh.serve: refresh closed, exiting...done")
	}
}

// hasExited is used for testing to check the goroutine has exited.
func (r *Refresh) hasExited() bool {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.exited
}

func newUID() string {
	const cooldown = 5 * time.Second
	for {
		k, err := ksuid.NewRandom()
		if err != nil {
			log.Printf("newUID: ksuid: %v, sleeping for %v", err, cooldown)
			time.Sleep(cooldown)
			continue
		}
		return k.String()
	}
}

// serve one amqp connection
func serveOnce(r *Refresh, connCount int, begin time.Time) {

	const me = "refresh.serveOne"

	exchangeName := "springCloudBus"
	exchangeType := "topic"
	queue := "kubeconfigserver." + newUID()

	log.Printf("%s: connection count:    %d", me, connCount)
	log.Printf("%s: amqp URL:            %s", me, r.amqpURL)
	log.Printf("%s: exchangeName:        %s", me, exchangeName)
	log.Printf("%s: exchangeType:        %s", me, exchangeType)
	log.Printf("%s: queue:               %s", me, queue)
	log.Printf("%s: consumerTag:         %s", me, r.consumerTag)
	log.Printf("%s: applications:        %v", me, r.applications)
	log.Printf("%s: dial timeout:        %v", me, r.dialTimeout)
	log.Printf("%s: dial retry interval: %v", me, r.dialRetryInterval)
	log.Printf("%s: debug:               %t", me, r.debug)

	conn := r.amqpClient.dial(r.isClosed, r.amqpURL, r.dialRetryInterval, r.dialTimeout)
	if conn == nil {
		log.Printf("%s: dial failed, refresh must have been closed by caller", me)
		return
	}
	defer r.amqpClient.closeConn(conn)

	ch, err := r.amqpClient.channel(conn)
	if err != nil {
		log.Printf("%s: failed to open channel: %v", me, err)
		return
	}
	defer r.amqpClient.closeChannel(ch)

	{
		log.Printf("%s: got channel, declaring exchange: exchangeName=%s exchangeType=%s", me, exchangeName, exchangeType)
		err := r.amqpClient.exchangeDeclare(ch, exchangeName, exchangeType)
		if err != nil {
			log.Printf("%s: failed to declare exchange: %v", me, err)
			return
		}
	}

	log.Printf("%s: declared exchange, declaring queue: %s", me, queue)
	q, err := r.amqpClient.queueDeclare(ch, queue)
	if err != nil {
		log.Printf("%s: failed to declare queue: %v", me, err)
		return
	}

	{
		const routingKey = "#"
		log.Printf("%s: declared queue (%d messages, %d consumers), binding to exchange '%s' with binding key '%s'",
			me, q.Messages, q.Consumers, exchangeName, routingKey)
		err := r.amqpClient.queueBind(ch, q.Name, routingKey, exchangeName)
		if err != nil {
			log.Printf("%s: failed to bind queue to exchange: %v", me, err)
			return
		}
	}

	if r.debug {
		log.Printf("DEBUG %s: entering consume loop", me)
	}

	// consume loop
	for consumes := 1; !r.isClosed(); consumes++ {
		msgs, err := r.amqpClient.consume(ch, q.Name, r.consumerTag)
		if err != nil {
			log.Printf("%s: conn=%d uptime=%v consume=%d: failed to register consumer: %v",
				me, connCount, time.Since(begin), consumes, err)
			return
		}
		var msg int

		if r.debug {
			log.Printf("DEBUG %s: entering delivery loop", me)
		}

	DELIVERY_LOOP:
		for {
			select {
			case d, ok := <-msgs:
				if !ok {
					log.Printf("%s: conn=%d uptime=%v consume=%d: consume channel closed", me, connCount, time.Since(begin), consumes)
					return
				}
				msg++
				if r.debug {
					log.Printf(
						"DEBUG %s: conn=%d uptime=%v consume=%d msg=%d: ConsumerTag=[%s] DeliveryTag=[%v] RoutingKey=[%s] Body='%s'",
						me,
						connCount,
						time.Since(begin),
						consumes,
						msg,
						d.ConsumerTag,
						d.DeliveryTag,
						d.RoutingKey,
						d.Body)
				}
				handleDelivery(r.isClosed, d.Body, r.applications, r.C, r.done, r.debug)
			case <-r.done:
				if r.debug {
					log.Printf("DEBUG %s: done channel has been closed", me)
				}
				r.amqpClient.cancel(ch, r.consumerTag)
				break DELIVERY_LOOP
			}
		}

		log.Printf("%s: conn=%d uptime=%v consume=%d: isClosed()=true", me, connCount, time.Since(begin), consumes)
	}
}

/*
	{
	    "type": "RefreshRemoteApplicationEvent",
	    "timestamp": 1649804650957,
	    "originService": "config-server:0:0a36277496365ee8621ae8f3ce7032ce",
	    "destinationService": "config-cli-example:**",
	    "id": "5a4cb501-652a-4ae2-9d3e-279e1d2a2169"
	}
*/
func handleDelivery(isClosed func() bool, body []byte, applications []string, ch chan<- string, done <-chan struct{}, debug bool) {
	const me = "handleDelivery"

	event := map[string]interface{}{}

	err := json.Unmarshal(body, &event)
	if err != nil {
		log.Printf("%s: body json error: %v", me, err)
		return
	}

	et := event["type"]
	eventType, typeIsStr := et.(string)
	if !typeIsStr {
		log.Printf("%s: 'type' is not a string: type=%[2]T value=%[2]v", me, et)
		return
	}

	if eventType != "RefreshRemoteApplicationEvent" {
		if debug {
			log.Printf("DEBUG %s: ignoring event type=[%s]", me, eventType)
		}
		return
	}

	destinationService := event["destinationService"]
	dst, isStr := destinationService.(string)
	if !isStr {
		log.Printf("%s: 'destinationService' is not a string: type=%[2]T value=%[2]v", me, destinationService)
		return
	}

	select {
	case ch <- dst:
	case <-done:
		if debug {
			log.Printf("DEBUG %s: done channel has been closed", me)
		}
	}
}
