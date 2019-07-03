package rediswatcher

import "github.com/garyburd/redigo/redis"

type WatcherOptions struct {
	Channel   string
	PubConn   redis.Conn
	SubConn   redis.Conn
	Password  string
	Protocol  string
	Committed chan struct{} // for transaction, we should do: Committed <- struct{}{} after transaction is committed.
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

func withRedisPubConnection(connection redis.Conn) WatcherOption {
	return func(options *WatcherOptions) {
		options.PubConn = connection
	}
}
