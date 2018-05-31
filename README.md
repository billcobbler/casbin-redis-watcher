Redis Watcher [![Build Status](https://travis-ci.org/billcobbler/casbin-redis-watcher.png?branch=master)](https://travis-ci.org/billcobbler/casbin-redis-watcher)
====

Redis Watcher is a [Redis](http://redis.io) watcher for [Casbin](https://github.com/casbin/casbin).

## Installation

    go get github.com/billcobbler/casbin-redis-watcher

## Simple Example

```go
package main

import (
    "github.com/casbin/casbin"
    "github.com/casbin/casbin/util"
    "github.com/billcobbler/casbin-redis-watcher"
)

func updateCallback(msg string) {
    util.LogPrint(msg)
}

func main() {
    // Initialize the watcher.
    // Use the Redis host as parameter.
    w, _ := rediswatcher.NewWatcher("127.0.0.1:6379")
    
    // Initialize the enforcer.
    e := casbin.NewEnforcer("examples/rbac_model.conf", "examples/rbac_policy.csv")
    
    // Set the watcher for the enforcer.
    e.SetWatcher(w)
    
    // Set callback to local example
    w.SetUpdateCallback(updateCallback)
    
    // Update the policy to test the effect.
    // You should see "[casbin rules updated]" in the log.
    e.SavePolicy()
}
```

## Getting Help

- [Casbin](https://github.com/casbin/casbin)

## License

This project is under Apache 2.0 License. See the [LICENSE](LICENSE) file for the full license text.