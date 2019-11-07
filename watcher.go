package rediswatcher

import (
	"runtime"
	"sync"
	"time"

	"fmt"

	"github.com/casbin/casbin/v2/persist"
	"github.com/garyburd/redigo/redis"
	"github.com/google/uuid"
)

type Watcher struct {
	options         WatcherOptions
	pubConn         redis.Conn
	subConn         redis.Conn
	callback        func(string)
	metricsCallback func(*WatcherMetrics)
	closed          chan struct{}
	messagesIn      chan redis.Message
	once            sync.Once
}

type WatcherMetrics struct {
	Name      string
	LatencyMs float64
	LocalID   string
	Channel   string
	Protocol  string
}

const (
	publishMetric   = "publish"
	subscribeMetric = "subscribe"
)

const (
	defaultShortMessageInTimeout = 1 * time.Millisecond
	defaultLongMessageInTimeout  = 1 * time.Minute
)

// NewWatcher creates a new Watcher to be used with a Casbin enforcer
// addr is a redis target string in the format "host:port"
// setters allows for inline WatcherOptions
//
// 		Example:
// 				w, err := rediswatcher.NewWatcher("127.0.0.1:6379", rediswatcher.Password("pass"), rediswatcher.Channel("/yourchan"))
//
// A custom redis.Conn can be provided to NewWatcher
//
// 		Example:
// 				c, err := redis.Dial("tcp", ":6379")
// 				w, err := rediswatcher.NewWatcher("", rediswatcher.WithRedisConnection(c)
//
func NewWatcher(addr string, setters ...WatcherOption) (persist.Watcher, error) {
	w := &Watcher{
		closed:     make(chan struct{}),
		messagesIn: make(chan redis.Message),
	}

	w.options = WatcherOptions{
		Channel:            "/casbin",
		Protocol:           "tcp",
		LocalID:            uuid.New().String(),
		SquashTimeoutShort: defaultShortMessageInTimeout,
		SquashTimeoutLong:  defaultLongMessageInTimeout,
	}

	for _, setter := range setters {
		setter(&w.options)
	}

	if err := w.connect(addr); err != nil {
		return nil, err
	}

	// call destructor when the object is released
	runtime.SetFinalizer(w, finalizer)

	w.messageInProcessor()

	go func() {
		for {
			select {
			case <-w.closed:
				return
			default:
				err := w.subscribe()
				if err != nil {
					fmt.Printf("Failure from Redis subscription: %v", err)
				}
			}
		}
	}()

	return w, nil
}

// NewPublishWatcher return a Watcher only publish but not subscribe
func NewPublishWatcher(addr string, setters ...WatcherOption) (persist.Watcher, error) {
	w := &Watcher{
		closed: make(chan struct{}),
	}

	w.options = WatcherOptions{
		Channel:            "/casbin",
		Protocol:           "tcp",
		LocalID:            uuid.New().String(),
		SquashTimeoutShort: defaultShortMessageInTimeout,
		SquashTimeoutLong:  defaultLongMessageInTimeout,
	}

	for _, setter := range setters {
		setter(&w.options)
	}

	if err := w.connect(addr); err != nil {
		return nil, err
	}

	// call destructor when the object is released
	runtime.SetFinalizer(w, finalizer)

	return w, nil
}

// SetUpdateCallBack sets the update callback function invoked by the watcher
// when the policy is updated. Defaults to Enforcer.LoadPolicy()
func (w *Watcher) SetUpdateCallback(callback func(string)) error {
	w.callback = callback
	return nil
}

// SetMetricsCallback sets the callback function to be called if performance
// metric collection is enabled.
func (w *Watcher) SetMetricsCallback(callback func(*WatcherMetrics)) error {
	w.metricsCallback = callback
	return nil
}

// Update publishes a message to all other casbin instances telling them to
// invoke their update callback
func (w *Watcher) Update() error {
	startTime := time.Now()
	if _, err := w.pubConn.Do("PUBLISH", w.options.Channel, w.options.LocalID); err != nil {
		return err
	}
	if w.options.RecordMetrics {
		watcherMetrics := w.createMetrics(publishMetric)
		watcherMetrics.LatencyMs = float64(time.Since(startTime)) / float64(time.Millisecond)
		w.metricsCallback(watcherMetrics)
	}

	return nil
}

// Close disconnects the watcher from redis
func (w *Watcher) Close() {
	finalizer(w)
}

func (w *Watcher) connect(addr string) error {
	if err := w.connectPub(addr); err != nil {
		return err
	}

	if err := w.connectSub(addr); err != nil {
		return err
	}

	return nil
}

func (w *Watcher) connectPub(addr string) error {
	if w.options.PubConn != nil {
		w.pubConn = w.options.PubConn
		return nil
	}

	c, err := redis.Dial(w.options.Protocol, addr)
	if err != nil {
		return err
	}

	if w.options.Password != "" {
		_, err := c.Do("AUTH", w.options.Password)
		if err != nil {
			c.Close()
			return err
		}
	}

	w.pubConn = c
	return nil
}

func (w *Watcher) connectSub(addr string) error {
	if w.options.SubConn != nil {
		w.subConn = w.options.SubConn
		return nil
	}

	c, err := redis.Dial(w.options.Protocol, addr)
	if err != nil {
		return err
	}

	if w.options.Password != "" {
		_, err := c.Do("AUTH", w.options.Password)
		if err != nil {
			c.Close()
			return err
		}
	}

	w.subConn = c
	return nil
}

func (w *Watcher) subscribe() error {
	psc := redis.PubSubConn{Conn: w.subConn}

	if err := psc.Subscribe(w.options.Channel); err != nil {
		return err
	}
	defer psc.Unsubscribe()

	if w.options.RecordMetrics {
		w.metricsCallback(w.createMetrics(subscribeMetric))
	}
	for {
		msg := psc.Receive()
		switch n := msg.(type) {
		case error:
			return n
		case redis.Message:
			w.messagesIn <- msg.(redis.Message)
		case redis.Subscription:
			if n.Count == 0 {
				return nil
			}
		}

	}
}

func (w *Watcher) messageInProcessor() {
	doCallback := false
	var data string
	timeOut := w.options.SquashTimeoutLong
	go func() {
		for {
			select {
			case <-w.closed:
				return
			case msg := <-w.messagesIn:
				if w.callback != nil {
					data = string(msg.Data)

					switch {
					case !w.options.IgnoreSelf && !w.options.SquashMessages:
						w.callback(data)
					case w.options.IgnoreSelf && data == w.options.LocalID: // ignore message
					case !w.options.IgnoreSelf && w.options.SquashMessages:
						doCallback = true
					case w.options.IgnoreSelf && data != w.options.LocalID && !w.options.SquashMessages:
						w.callback(data)
					case w.options.IgnoreSelf && data != w.options.LocalID && w.options.SquashMessages:
						doCallback = true
					default:
						w.callback(data)
					}
				}
				if doCallback { // set short timeout
					timeOut = w.options.SquashTimeoutShort
				}
			case <-time.After(timeOut):
				if doCallback {
					w.callback(data) // data will be last message recieved
					doCallback = false
					timeOut = w.options.SquashTimeoutLong // long timeout
				}
			}
		}
	}()
}

func (w *Watcher) createMetrics(metricsName string) *WatcherMetrics {
	return &WatcherMetrics{
		Name:     metricsName,
		Channel:  w.options.Channel,
		LocalID:  w.options.LocalID,
		Protocol: w.options.Protocol,
	}
}

// return option settings
func (w *Watcher) GetWatcherOptions() WatcherOptions {
	return w.options
}

func finalizer(w *Watcher) {
	w.once.Do(func() {
		close(w.closed)
		w.subConn.Close()
		w.pubConn.Close()
	})
}
