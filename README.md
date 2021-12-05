# goll-redis

[![Documentation](https://godoc.org/github.com/fabiofenoglio/goll-redis?status.svg)](http://godoc.org/github.com/fabiofenoglio/goll-redis)
[![Go Report Card](https://goreportcard.com/badge/github.com/fabiofenoglio/goll-redis)](https://goreportcard.com/report/github.com/fabiofenoglio/goll-redis)

goll-redis is a sample synchronization adapter for [Goll](https://github.com/fabiofenoglio/goll) using redis.

It depends on [Redsync](https://github.com/go-redsync) as distributed lock implementation over Redis

- [Installation](#installation)
- [Quickstart](#quickstart)
	- [Create a Redis pool](#create-a-redis-pool)
	- [Create the adapter](#create-the-adapter)
	- [Create a LoadLimiter instance with the adapter](#create-a-loadlimiter-instance-with-the-adapter)
- [Full example](#full-example)

## Installation

```
go get github.com/fabiofenoglio/goll-redis
```

## Quickstart

The implementation requires you to bring your own Redis pool so that you can 
plug in your preferred Redis client library.

You may use `go-redis` for instance:

```bash
go get github.com/go-redis/redis/v8
```

### Create a Redis pool

Remember to import goll, the goll-redis adapter **and** the client library for Redis.

Your imports should be similar to these:

```go
import (
	goll "github.com/fabiofenoglio/goll"
	gollredis "github.com/fabiofenoglio/goll-redis"
	goredislib "github.com/go-redis/redis/v8"
	"github.com/go-redsync/redsync/v4/redis/goredis/v8"
)
```

You can start by creating a Redis client and a pool:


```go
client := goredislib.NewClient(&goredislib.Options{
    Addr:     "localhost:6379",
})

pool := goredis.NewPool(client)
```

### Create the adapter

Use the pool you just created to get an instance of the goll-redis adapter:

```go
adapter, err := gollredis.NewRedisSyncAdapter(&gollredis.Config{
    Pool:      pool,
    MutexName: "redisAdapterTest",
})
```

### Create a LoadLimiter instance with the adapter

Now just pass the adapter to the `New` method and your load limiter will synchronize with other identical instances connected to the same Redis instance:

```go
limiter, err := goll.New(&goll.Config{
    MaxLoad:           1000,
    WindowSize:        20 * time.Second,
    SyncAdapter:       adapter, // the adapter we just created
})
```

You can now use your instance and enjoy automatic synchronization.

You may notice some new logs regarding sync transactions, look up for any warnings or errors because by default synchronization errors are not blocking.

```
...
2021/11/30 18:04:19 [info] [sync tx] acquiring lock
2021/11/30 18:04:19 [info] [sync tx] lock acquired
2021/11/30 18:04:19 [info] [sync tx] fetching status
2021/11/30 18:04:19 [info] [sync tx] fetched status
2021/11/30 18:04:19 [info] instance version is up to date with serialized data, nothing to do
2021/11/30 18:04:19 [info] [sync tx] executing task
2021/11/30 18:04:19 [info] [sync tx] writing updated status to remote store
2021/11/30 18:04:19 [info] [sync tx] end
2021/11/30 18:04:19 [info] [sync tx] releasing lock
2021/11/30 18:04:19 [info] [sync tx] lock released
request for load of 17 was accepted
2021/11/30 18:04:19 [info] [sync tx] acquiring lock
2021/11/30 18:04:19 [info] [sync tx] lock acquired
2021/11/30 18:04:19 [info] [sync tx] fetching status
2021/11/30 18:04:19 [info] [sync tx] fetched status
2021/11/30 18:04:19 [info] instance version is up to date with serialized data, nothing to do
2021/11/30 18:04:19 [info] [sync tx] executing task
2021/11/30 18:04:19 [info] [sync tx] end
2021/11/30 18:04:19 [info] [sync tx] releasing lock
2021/11/30 18:04:19 [info] [sync tx] lock released
limiter status: windowTotal=28 segments=[ 17  11], 2 requests processed
...
```

## Full example

```go
package main

import (
	"fmt"
	"math/rand"
	"time"

	goll "github.com/fabiofenoglio/goll"
	gollredis "github.com/fabiofenoglio/goll-redis"
	goredislib "github.com/go-redis/redis/v8"
	"github.com/go-redsync/redsync/v4/redis/goredis/v8"
)

func main() {
	// Create a pool with go-redis (or redigo) which is the pool redisync will
	// use while communicating with Redis. This can also be any pool that
	// implements the `redis.Pool` interface.
	client := goredislib.NewClient(&goredislib.Options{
		// set your redis connection parameters here.
		Addr: "localhost:6379",
	})
	defer client.Close()

	pool := goredis.NewPool(client)

	adapter, err := gollredis.NewRedisSyncAdapter(&gollredis.Config{
		Pool:      pool,
		MutexName: "redisAdapterTest",
	})

	if err != nil {
		panic(err)
	}

	// create an instance of LoadLimiter
	// accepting a max of 1000 over 20 seconds
	newLimiter, _ := goll.New(&goll.Config{
		MaxLoad:           1000,
		WindowSize:        20 * time.Second,
		SyncAdapter:       adapter,
	})

	// not caring for multi-tenancy right now,
	// so let's switch to a single-tenant interface
	limiter := newLimiter.ForTenant("test")

	// we'll gather some stats to see how our boy performs
	startedAt := time.Now()
	requested := uint64(0)
	accepted := uint64(0)

	for i := 0; i < 100; i++ {
		// require a random amount of load from 10 to 50
		// to simulate various kind of requests
		requestedLoad := uint64(rand.Intn(50-10) + 10)

		requested += requestedLoad

		// submit the request to the limiter
		submitResult, err := limiter.Submit(requestedLoad)
		if err != nil {
			panic(fmt.Errorf("error submitting: %w", err))
		}

		if submitResult.Accepted {
			accepted += requestedLoad
			fmt.Printf("request for load of %v was accepted\n", requestedLoad)

		} else if submitResult.RetryInAvailable {
			fmt.Printf("request for load of %v was rejected, asked to wait for %v ms\n", requestedLoad, submitResult.RetryIn.Milliseconds())

			// wait, then resubmit
			time.Sleep(submitResult.RetryIn)

			requested += requestedLoad
			submitResult, err = limiter.Submit(requestedLoad)

			if err != nil {
				panic(fmt.Errorf("error resubmitting: %w", err))
			}
			if submitResult.Accepted {
				fmt.Printf("resubmitted request for load of %v was accepted\n", requestedLoad)
				accepted += requestedLoad
			} else {
				panic("waited the required amount of time but the request was rejected again :(")
			}

		} else {
			fmt.Printf("request for load of %v was rejected with no indications on the required delay before resubmitting\n", requestedLoad)
		}

		// sleep for a random amount of time from 0 to 1000ms
		// to simulate random requests pattern
		time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
	}

	// check if the limiter really did limit.
	// we expect a "total accepted" around 50/sec (1000 over 20 seconds)
	demoDuration := time.Now().Unix() - startedAt.Unix()
	fmt.Printf("**********************************\n")
	fmt.Printf("total duration: %d sec\n", demoDuration)
	fmt.Printf("total accepted: %.2v ( %.2f/sec )\n", accepted, float64(accepted)/float64(demoDuration))

}
```