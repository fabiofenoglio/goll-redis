package gollredis

import (
	"testing"

	"github.com/fabiofenoglio/goll"
)

func TestInterfacesAreCorrectlyImplemented(t *testing.T) {

	adapter, _ := NewRedisSyncAdapter(&Config{
		Pool:      nil,
		MutexName: "redisAdapterTest",
	})

	acceptor := func(i goll.SyncAdapter) {}
	acceptor(adapter)
}
