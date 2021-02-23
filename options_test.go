package rediswatcher

import "testing"

func TestOptions(t *testing.T) {
	o := WatcherOptions{
		Channel:  "ch1",
		Password: "pa1",
		Protocol: "pr1",
		Username: "user1",
	}

	// test single option
	o.optionBuilder(Channel("ch2"))

	if o.Channel != "ch2" {
		t.Errorf("Channel should be 'ch2', received '%s' instead", o.Channel)
	}

	if o.Username != "user1" {
		t.Errorf("Username should be 'user1', received '%s' instead", o.Username)
	}

	if o.Password != "pa1" {
		t.Errorf("Password should be 'pa1', received '%s' instead", o.Password)
	}

	// test multiple options
	o.optionBuilder(Channel("ch3"), Password("pa3"), Protocol("pr3"), Username("user3"))

	if o.Channel != "ch3" {
		t.Errorf("Channel should be 'ch3', received '%s' instead", o.Channel)
	}

	if o.Password != "pa3" {
		t.Errorf("Password should be 'pa3', received '%s' instead", o.Password)
	}

	if o.Protocol != "pr3" {
		t.Errorf("Protocol should be 'pr3', received '%s' instead", o.Password)
	}

	if o.Username != "user3" {
		t.Errorf("Username should be 'user3', received '%s' instead", o.Username)
	}
}

func (o *WatcherOptions) optionBuilder(setters ...WatcherOption) {
	for _, setter := range setters {
		setter(o)
	}
}
