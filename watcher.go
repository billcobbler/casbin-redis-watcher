package rediswatcher

import (
	"runtime"
	"sync"

	"fmt"

	"github.com/casbin/casbin/v2/persist"
	"github.com/garyburd/redigo/redis"
	"github.com/google/uuid"
)

type Watcher struct {
	options  WatcherOptions
	pubConn  redis.Conn
	subConn  redis.Conn
	callback func(string)
	closed   chan struct{}
	once     sync.Once
}

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
		closed: make(chan struct{}),
	}

	w.options = WatcherOptions{
		Channel:  "/casbin",
		Protocol: "tcp",
		LocalID:  uuid.New().String(),
	}

	for _, setter := range setters {
		setter(&w.options)
	}

	if err := w.connect(addr); err != nil {
		return nil, err
	}

	// call destructor when the object is released
	runtime.SetFinalizer(w, finalizer)

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
		Channel:  "/casbin",
		Protocol: "tcp",
		LocalID:  uuid.New().String(),
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
	if _, err := w.pubConn.Do("PUBLISH", w.options.Channel, w.options.LocalID); err != nil {
		return err
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

func (w *Watcher) getMessages(psc *redis.PubSubConn) []interface{} {
	messages := make([]interface{}, 1)
	messages[0] = psc.Receive()
	// only return 1 message at a time if SquashMessages not enabled
	if !w.options.SquashMessages {
		return messages
	}
	for {
		if !psc.Peek() {
			return messages
		}
		msg := psc.Receive()
		if msg != nil {
			switch msg.(type) {
			case redis.Message:
				messages = append(messages, msg)
			case error:
				messages = append(messages, msg)
				return messages
			default:
				messages = append(messages, msg)
			}
		} else {
			break
		}
	}
	return messages
}

func (w *Watcher) subscribe() error {
	psc := redis.PubSubConn{Conn: w.subConn}

	if err := psc.Subscribe(w.options.Channel); err != nil {
		return err
	}
	defer psc.Unsubscribe()

	for {
		doCallback := false
		var data string
		messages := w.getMessages(&psc) // get all available messages
		for _, msg := range messages {
			switch n := msg.(type) {
			case error:
				return n
			case redis.Message:
				if w.callback != nil {
					data = string(n.Data)
					if !w.options.IgnoreSelf || (w.options.IgnoreSelf && data != w.options.LocalID) {
						doCallback = true
					}
				}
			case redis.Subscription:
				if n.Count == 0 {
					return nil
				}
			}
		}
		if doCallback {
			w.callback(data) // data will be last message recieved
		}
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
