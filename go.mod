module github.com/lucidhq/casbin-redis-watcher/v2

go 1.12

replace github.com/garyburd/redigo => github.com/gomodule/redigo v1.7.0

require (
	github.com/casbin/casbin/v2 v2.0.1
	github.com/garyburd/redigo v1.6.0
	github.com/google/uuid v1.1.1
	github.com/rafaeljusto/redigomock v0.0.0-20170720131524-7ae0511314e9
)
