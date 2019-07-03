package rediswatcher

import (
	"runtime"

	"fmt"
	"github.com/casbin/casbin/persist"
	"github.com/garyburd/redigo/redis"
)

type Watcher struct {
	options  WatcherOptions
	pubConn  redis.Conn
	subConn  redis.Conn
	callback func(string)
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
	w := &Watcher{}

	w.options = WatcherOptions{
		Channel:  "/casbin",
		Protocol: "tcp",
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
			err := w.subscribe()
			if err != nil {
				fmt.Printf("Failure from Redis subscription: %v", err)
			}
		}
	}()

	return w, nil
}

// NewPublishWatcher return a Watcher only publish but not subscribe
func NewPublishWatcher(addr string, setters ...WatcherOption) (persist.Watcher, error) {
	w := &Watcher{}

	w.options = WatcherOptions{
		Channel:  "/casbin",
		Protocol: "tcp",
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
	if w.options.Committed == nil {
		if _, err := w.pubConn.Do("PUBLISH", w.options.Channel, "casbin rules updated"); err != nil {
			return err
		}
	} else {
		// use transaction. new a goroutine avoid to block
		// the error of Update has been intentionally ignored. so it won't make anything wrong if we don't return error
		go func() {
			select {
			case <-w.options.Committed:
				_, _ = w.pubConn.Do("PUBLISH", w.options.Channel, "casbin rules updated")
			}
		}()
	}

	return nil
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

	for {
		switch n := psc.Receive().(type) {
		case error:
			return n
		case redis.Message:
			if w.callback != nil {
				w.callback(string(n.Data))
			}
		case redis.Subscription:
			if n.Count == 0 {
				return nil
			}
		}
	}

	return nil
}

func finalizer(w *Watcher) {
	w.subConn.Close()
	w.pubConn.Close()
}
