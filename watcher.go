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
	options    WatcherOptions
	pubConn    redis.Conn
	subConn    redis.Conn
	callback   func(string)
	closed     chan struct{}
	messagesIn chan redis.Message
	once       sync.Once
}

type WatcherMetrics struct {
	Name        string
	LatencyMs   float64
	LocalID     string
	Channel     string
	Protocol    string
	Error       error
	MessageSize int64
}

const (
	RedisDoAuthMetric       = "RedisDoAuth"
	RedisCloseMetric        = "RedisClose"
	RedisDialMetric         = "RedisDial"
	PubSubPublishMetric     = "PubSubPublish"
	PubSubReceiveMetric     = "PubSubReceive"
	PubSubSubscribeMetric   = "PubSubSubscribe"
	PubSubUnsubscribeMetric = "PubSubUnsubscribe"
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
				err := w.connect(addr)
				if err == nil {
					err = w.subscribe()
				}
				if err != nil {
					fmt.Printf("Failure from Redis subscription: %v\n", err)
				}
				time.Sleep(2 * time.Second)
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

// Update publishes a message to all other casbin instances telling them to
// invoke their update callback
func (w *Watcher) Update() error {
	startTime := time.Now()
	if _, err := w.pubConn.Do("PUBLISH", w.options.Channel, w.options.LocalID); err != nil {
		if w.options.RecordMetrics != nil {
			w.options.RecordMetrics(w.createMetrics(PubSubPublishMetric, startTime, err))
		}
		return err
	}
	if w.options.RecordMetrics != nil {
		w.options.RecordMetrics(w.createMetrics(PubSubPublishMetric, startTime, nil))
	}

	return nil
}

// Close disconnects the watcher from redis
func (w *Watcher) Close() {
	finalizer(w)
}

func (w *Watcher) connect(addr string) error {
	var pubConnErr error
	if w.pubConn != nil {
		pubConnErr = w.pubConn.Err()
	}
	if w.pubConn == nil || pubConnErr != nil {
		if err := w.connectPub(addr); err != nil {
			return err
		}
	}

	var subConnErr error
	if w.subConn != nil {
		subConnErr = w.subConn.Err()
	}
	if w.subConn == nil || subConnErr != nil {
		if err := w.connectSub(addr); err != nil {
			return err
		}
	}

	return nil
}

func (w *Watcher) connectPub(addr string) error {
	if w.options.PubConn != nil {
		w.pubConn = w.options.PubConn
		return nil
	}

	c, err := w.dial(addr)
	if err != nil {
		return err
	}
	w.pubConn = *c
	return nil
}

func (w *Watcher) connectSub(addr string) error {
	if w.options.SubConn != nil {
		w.subConn = w.options.SubConn
		return nil
	}

	c, err := w.dial(addr)
	if err != nil {
		return err
	}
	w.subConn = *c
	return nil
}

func (w *Watcher) dial(addr string) (*redis.Conn, error) {
	startTime := time.Now()
	c, err := redis.Dial(w.options.Protocol, addr)
	if err != nil {
		if w.options.RecordMetrics != nil {
			w.options.RecordMetrics(w.createMetrics(RedisDialMetric, startTime, err))
		}
		return nil, err
	}
	if w.options.RecordMetrics != nil {
		w.options.RecordMetrics(w.createMetrics(RedisDialMetric, startTime, nil))
	}
	if w.options.Password != "" {
		startTime = time.Now()
		_, err = c.Do("AUTH", w.options.Password)
		if err != nil {
			if w.options.RecordMetrics != nil {
				w.options.RecordMetrics(w.createMetrics(RedisDoAuthMetric, startTime, err))
			}
			startTime = time.Now()
			err2 := c.Close()
			if w.options.RecordMetrics != nil {
				w.options.RecordMetrics(w.createMetrics(RedisCloseMetric, startTime, err2))
			}
			return nil, err
		}
		if w.options.RecordMetrics != nil {
			w.options.RecordMetrics(w.createMetrics(RedisDoAuthMetric, startTime, nil))
		}
	}
	return &c, nil
}

func (w *Watcher) unsubscribe(psc redis.PubSubConn) {
	startTime := time.Now()
	err := psc.Unsubscribe()
	if w.options.RecordMetrics != nil {
		w.options.RecordMetrics(w.createMetrics(PubSubUnsubscribeMetric, startTime, err))
	}
}

func (w *Watcher) subscribe() error {
	psc := redis.PubSubConn{Conn: w.subConn}
	startTime := time.Now()
	if err := psc.Subscribe(w.options.Channel); err != nil {
		if w.options.RecordMetrics != nil {
			w.options.RecordMetrics(w.createMetrics(PubSubSubscribeMetric, startTime, err))
		}
		return err
	}
	if w.options.RecordMetrics != nil {
		w.options.RecordMetrics(w.createMetrics(PubSubSubscribeMetric, startTime, nil))
	}
	defer w.unsubscribe(psc)

	for {
		startTime := time.Now()
		msg := psc.Receive()
		switch n := msg.(type) {
		case error:
			if w.options.RecordMetrics != nil {
				w.options.RecordMetrics(w.createMetrics(PubSubReceiveMetric, startTime, n))
			}
			return n
		case redis.Message:
			if w.options.RecordMetrics != nil {
				watcherMetrics := w.createMetrics(PubSubReceiveMetric, startTime, nil)
				watcherMetrics.MessageSize = int64(len(n.Data))
				w.options.RecordMetrics(watcherMetrics)
			}
			w.messagesIn <- msg.(redis.Message)
		case redis.Subscription:
			if w.options.RecordMetrics != nil {
				w.options.RecordMetrics(w.createMetrics(PubSubReceiveMetric, startTime, nil))
			}
			if n.Count == 0 {
				return nil
			}
		}

	}
}

func (w *Watcher) messageInProcessor() {
	w.options.callbackPending = false
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
						w.options.callbackPending = true
					case w.options.IgnoreSelf && data != w.options.LocalID && !w.options.SquashMessages:
						w.callback(data)
					case w.options.IgnoreSelf && data != w.options.LocalID && w.options.SquashMessages:
						w.options.callbackPending = true
					default:
						w.callback(data)
					}
				}
				if w.options.callbackPending { // set short timeout
					timeOut = w.options.SquashTimeoutShort
				}
			case <-time.After(timeOut):
				if w.options.callbackPending {
					w.options.callbackPending = false
					w.callback(data)                      // data will be last message recieved
					timeOut = w.options.SquashTimeoutLong // long timeout
				}
			}
		}
	}()
}

func (w *Watcher) createMetrics(metricsName string, startTime time.Time, err error) *WatcherMetrics {
	return &WatcherMetrics{
		Name:      metricsName,
		Channel:   w.options.Channel,
		LocalID:   w.options.LocalID,
		Protocol:  w.options.Protocol,
		LatencyMs: float64(time.Since(startTime)) / float64(time.Millisecond),
		Error:     err,
	}
}

// return option settings
func (w *Watcher) GetWatcherOptions() WatcherOptions {
	return w.options
}

func finalizer(w *Watcher) {
	w.once.Do(func() {
		close(w.closed)
		startTime := time.Now()
		err := w.subConn.Close()
		if w.options.RecordMetrics != nil {
			w.options.RecordMetrics(w.createMetrics(RedisCloseMetric, startTime, err))
		}
		startTime = time.Now()
		err = w.pubConn.Close()
		if w.options.RecordMetrics != nil {
			w.options.RecordMetrics(w.createMetrics(RedisCloseMetric, startTime, err))
		}
	})
}
