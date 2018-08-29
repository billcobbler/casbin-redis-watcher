package rediswatcher

import (
	"github.com/casbin/casbin"
	"github.com/rafaeljusto/redigomock"
	"testing"
	"time"
)

func TestWatcher(t *testing.T) {
	if _, err := NewWatcher(""); err == nil {
		t.Error("Connecting to nothing should fail")
	}

	// setup mock redis
	c := redigomock.NewConn()
	c.Clear()
	c.ReceiveWait = true
	c.Command("PUBLISH", "/casbin", "casbin rules updated").Expect("1")

	values := []interface{}{}
	values = append(values, interface{}([]byte("subscribe")))
	values = append(values, interface{}([]byte("/casbin")))
	values = append(values, interface{}([]byte("1")))

	c.Command("SUBSCRIBE", "/casbin").Expect(values)

	w, err := NewWatcher("127.0.0.1:6379", WithRedisSubConnection(c), withRedisPubConnection(c))
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}

	if err := w.Update(); err != nil {
		t.Fatalf("Failed watcher.Update(): %v", err)
	}
}

func TestWithEnforcer(t *testing.T) {
	// setup mock redis
	c := redigomock.NewConn()
	c.Clear()
	c.ReceiveWait = true
	c.Command("PUBLISH", "/casbin", "casbin rules updated").Expect("1")

	unsubValues := []interface{}{}
	unsubValues = append(unsubValues, interface{}([]byte("unsubscribe")))
	unsubValues = append(unsubValues, interface{}(nil))
	unsubValues = append(unsubValues, interface{}([]byte("1")))
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

	w, err := NewWatcher("127.0.0.1:6379", WithRedisSubConnection(c), withRedisPubConnection(c))
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}

	e := casbin.NewEnforcer("examples/rbac_model.conf", "examples/rbac_policy.csv")

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
	case <-time.After(time.Second * 2):
		t.Fatal("Enforcer message timed out")
	}
}
