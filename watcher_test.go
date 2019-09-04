package rediswatcher

import (
	"testing"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/rafaeljusto/redigomock"
)

type testConn struct {
	redigomock.Conn
}

func NewTestConn() *testConn {
	tc := &testConn{*redigomock.NewConn()}
	return tc
}
func TestWatcher(t *testing.T) {
	if _, err := NewWatcher(""); err == nil {
		t.Error("Connecting to nothing should fail")
	}

	// setup mock redis
	c := NewTestConn()

	c.Clear()
	c.ReceiveWait = true

	values := []interface{}{}
	values = append(values, interface{}([]byte("subscribe")))
	values = append(values, interface{}([]byte("/casbin")))
	values = append(values, interface{}([]byte("1")))

	c.Command("SUBSCRIBE", "/casbin").Expect(values)

	w, err := NewWatcher("127.0.0.1:6379", WithRedisSubConnection(c), WithRedisPubConnection(c))
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}

	wi := w.(interface{})
	rediswatch := wi.(*Watcher)
	c.Command("PUBLISH", "/casbin", rediswatch.GetWatcherOptions().LocalID).Expect("1")

	if err := w.Update(); err != nil {
		t.Fatalf("Failed watcher.Update(): %v", err)
	}

	closed := false
	c.CloseMock = func() error {
		closed = true
		return nil
	}

	w.Close()
	if !closed {
		t.Fatal("watcher.Close() failed to close Redis connection")
	}

	// multiple closes should not panic
	w.Close()
}

func TestWithEnforcer(t *testing.T) {
	// setup mock redis
	c := NewTestConn()
	c.Clear()
	c.ReceiveWait = true
	c.Command("PUBLISH", "/casbin", "casbin rules updated").Expect("1")

	unsubValues := []interface{}{}
	unsubValues = append(unsubValues, interface{}([]byte("unsubscribe")))
	unsubValues = append(unsubValues, interface{}("a"))
	unsubValues = append(unsubValues, interface{}([]byte("0")))
	c.Command("UNSUBSCRIBE").Expect(unsubValues)

	subValues := []interface{}{}
	subValues = append(subValues, interface{}([]byte("subscribe")))
	subValues = append(subValues, interface{}([]byte("/casbin")))
	subValues = append(subValues, interface{}([]byte("1")))
	c.Command("SUBSCRIBE", "/casbin").Expect(subValues)

	values := []interface{}{}
	values = append(values, interface{}([]byte("message")))
	values = append(values, interface{}([]byte("/casbin")))
	values = append(values, interface{}([]byte("casbin rules updated")))
	c.AddSubscriptionMessage(values)

	w, err := NewWatcher("127.0.0.1:6379", WithRedisSubConnection(c), WithRedisPubConnection(c))
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}

	e, err := casbin.NewEnforcer("examples/rbac_model.conf", "examples/rbac_policy.csv")
	if err != nil {
		t.Fatalf("Failed to create enforcer: %v", err)
	}
	e.SetWatcher(w)

	ch := make(chan string, 1)
	w.SetUpdateCallback(func(msg string) {
		ch <- msg
	})

	e.SavePolicy()

	go func() {
		c.ReceiveNow <- true
	}()

	select {
	case res := <-ch:
		if res != "casbin rules updated" {
			t.Fatalf("Message should be 'casbin rules updated', received '%v' instead", res)
		}
	case <-time.After(time.Second * 5):
		t.Fatal("Enforcer message timed out")
	}
}

func TestWithEnforcerSquash(t *testing.T) {
	// setup mock redis
	c := NewTestConn()
	c.Clear()
	c.ReceiveWait = true
	c.Command("PUBLISH", "/casbin", "casbin rules updated").Expect("1")

	unsubValues := []interface{}{}
	unsubValues = append(unsubValues, interface{}([]byte("unsubscribe")))
	unsubValues = append(unsubValues, interface{}("a"))
	unsubValues = append(unsubValues, interface{}([]byte("0")))
	c.Command("UNSUBSCRIBE").Expect(unsubValues)

	subValues := []interface{}{}
	subValues = append(subValues, interface{}([]byte("subscribe")))
	subValues = append(subValues, interface{}([]byte("/casbin")))
	subValues = append(subValues, interface{}([]byte("1")))
	c.Command("SUBSCRIBE", "/casbin").Expect(subValues)

	w, err := NewWatcher("127.0.0.1:6379", WithRedisSubConnection(c), WithRedisPubConnection(c), SquashMessages(true))
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}

	values := []interface{}{}
	values = append(values, interface{}([]byte("message")))
	values = append(values, interface{}([]byte("/casbin")))
	wp := w.(interface{})
	rw := wp.(*Watcher)
	values = append(values, interface{}([]byte(rw.GetWatcherOptions().LocalID)))
	c.AddSubscriptionMessage(values)
	c.AddSubscriptionMessage(values)

	e, err := casbin.NewEnforcer("examples/rbac_model.conf", "examples/rbac_policy.csv")
	if err != nil {
		t.Fatalf("Failed to create enforcer: %v", err)
	}
	e.SetWatcher(w)

	ch := make(chan string, 2)
	w.SetUpdateCallback(func(msg string) {
		ch <- msg
	})

	e.SavePolicy()

	go func() {
		c.ReceiveNow <- true
		c.ReceiveNow <- true
	}()

	select {
	case res := <-ch:
		if res != rw.GetWatcherOptions().LocalID {
			t.Fatalf("Message should be '%s', received '%v' instead", rw.GetWatcherOptions().LocalID, res)
		}
	case <-time.After(time.Second * 5):
		t.Fatal("Enforcer message timed out")
	}
	// should timeout for this one
	select {
	case res := <-ch:
		t.Fatalf("Receieved message that should have been squashed.  Message received '%v'", res)
	case <-time.After(time.Millisecond * 50):
	}
}

func TestWithEnforcerIgnoreSelf(t *testing.T) {
	// setup mock redis
	c := NewTestConn()
	c.Clear()
	c.ReceiveWait = true
	c.Command("PUBLISH", "/casbin", "casbin rules updated").Expect("1")

	unsubValues := []interface{}{}
	unsubValues = append(unsubValues, interface{}([]byte("unsubscribe")))
	unsubValues = append(unsubValues, interface{}("a"))
	unsubValues = append(unsubValues, interface{}([]byte("0")))
	c.Command("UNSUBSCRIBE").Expect(unsubValues)

	subValues := []interface{}{}
	subValues = append(subValues, interface{}([]byte("subscribe")))
	subValues = append(subValues, interface{}([]byte("/casbin")))
	subValues = append(subValues, interface{}([]byte("1")))
	c.Command("SUBSCRIBE", "/casbin").Expect(subValues)

	w, err := NewWatcher("127.0.0.1:6379", WithRedisSubConnection(c), WithRedisPubConnection(c), IgnoreSelf(true))
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}

	values := []interface{}{}
	values = append(values, interface{}([]byte("message")))
	values = append(values, interface{}([]byte("/casbin")))
	wp := w.(interface{})
	rw := wp.(*Watcher)
	values = append(values, interface{}([]byte(rw.GetWatcherOptions().LocalID)))
	c.AddSubscriptionMessage(values)

	e, err := casbin.NewEnforcer("examples/rbac_model.conf", "examples/rbac_policy.csv")
	if err != nil {
		t.Fatalf("Failed to create enforcer: %v", err)
	}
	e.SetWatcher(w)

	ch := make(chan string, 2)
	w.SetUpdateCallback(func(msg string) {
		ch <- msg
	})

	e.SavePolicy()

	go func() {
		c.ReceiveNow <- true
	}()

	// should timeout for this one
	select {
	case res := <-ch:
		t.Fatalf("Receieved message that should have been ignored.  Message received '%v'", res)
	case <-time.After(time.Millisecond * 50):
	}
}
