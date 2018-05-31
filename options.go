package rediswatcher

import "github.com/garyburd/redigo/redis"

type WatcherOptions struct {
	Channel    string
	Connection redis.Conn
	Password   string
	Protocol   string
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

func WithRedisConnection(connection redis.Conn) WatcherOption {
	return func(options *WatcherOptions) {
		options.Connection = connection
	}
}
