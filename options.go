package rediswatcher

import "github.com/garyburd/redigo/redis"

type WatcherOptions struct {
	Channel    string
	PubConn    redis.Conn
	SubConn    redis.Conn
	Password   string
	Protocol   string
	IgnoreSelf bool
	LocalID    string
	SquashMessages bool
}

type WatcherOption func(*WatcherOptions)

func Channel(subject string) WatcherOption {
	return func(options *WatcherOptions) {
		options.Channel = subject
	}
}

func Password(password string) WatcherOption {
	return func(options *WatcherOptions) {
		options.Password = password
	}
}

func Protocol(protocol string) WatcherOption {
	return func(options *WatcherOptions) {
		options.Protocol = protocol
	}
}

func WithRedisSubConnection(connection redis.Conn) WatcherOption {
	return func(options *WatcherOptions) {
		options.SubConn = connection
	}
}

func WithRedisPubConnection(connection redis.Conn) WatcherOption {
	return func(options *WatcherOptions) {
		options.PubConn = connection
	}
}

func LocalID(id string) WatcherOption {
	return func(options *WatcherOptions) {
		options.LocalID = id
	}
}

func IgnoreSelf(ignore bool) WatcherOption {
	return func(options *WatcherOptions) {
		options.IgnoreSelf = ignore
	}
}

func SquashMessages(squash bool)  WatcherOption {
	return func(options *WatcherOptions) {
		options.SquashMessages = squash
	}
}