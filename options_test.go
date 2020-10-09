package rediswatcher

import (
	"fmt"
	"testing"
	"time"
)

func TestOptions(t *testing.T) {
	callbackInvoked := 0
	o := WatcherOptions{
		Channel:            "ch1",
		Password:           "pa1",
		Protocol:           "pr1",
		reconnectThreshold: 7 * time.Second,
		reconnectFailureCallback: func(err error) {
			t.Logf("received error: %+v\n", err)
			callbackInvoked = callbackInvoked + 1
		},
	}

	if o.reconnectThreshold != 7*time.Second {
		t.Errorf("Reconnect threshold should be '7s', received '%s' instead", o.Password)
	}

	o.reconnectFailureCallback(fmt.Errorf("test_error"))
	if callbackInvoked != 1 {
		t.Errorf("Reconnect failure callback not invoked")
	}

	// test single option
	o.optionBuilder(Channel("ch2"))

	if o.Channel != "ch2" {
		t.Errorf("Channel should be 'ch2', received '%s' instead", o.Channel)
	}

	if o.Password != "pa1" {
		t.Errorf("Password should be 'pa1', received '%s' instead", o.Password)
	}

	// test multiple options
	o.optionBuilder(Channel("ch3"), Password("pa3"), Protocol("pr3"), ReconnectThreshold(time.Second), ReconnectFailureCallback(func(err error) {
		t.Logf("received error #2: %+v\n", err)
		callbackInvoked = callbackInvoked + 9
	}))

	if o.Channel != "ch3" {
		t.Errorf("Channel should be 'ch3', received '%s' instead", o.Channel)
	}

	if o.Password != "pa3" {
		t.Errorf("Password should be 'pa3', received '%s' instead", o.Password)
	}

	if o.Protocol != "pr3" {
		t.Errorf("Protocol should be 'pr3', received '%s' instead", o.Password)
	}

	if o.reconnectThreshold != time.Second {
		t.Errorf("Reconnect threshold should be '1s', received '%s' instead", o.Password)
	}

	o.reconnectFailureCallback(fmt.Errorf("test_error"))
	if callbackInvoked != 10 {
		t.Errorf("Reconnect failure callback not invoked")
	}
}

func (o *WatcherOptions) optionBuilder(setters ...WatcherOption) {
	for _, setter := range setters {
		setter(o)
	}
}
