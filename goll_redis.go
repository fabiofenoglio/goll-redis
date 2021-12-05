package gollredis

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	goll "github.com/fabiofenoglio/goll"
	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis"
)

type Config struct {
	MutexName string
	Pool      redis.Pool
}

type redisSyncAdapter struct {
	pool        redis.Pool
	redsync     *redsync.Redsync
	localMutext sync.Mutex
	mutexMap    map[string]*redsync.Mutex
	mutexName   string
}

func NewRedisSyncAdapter(config *Config) (goll.SyncAdapter, error) {
	rs := redsync.New(config.Pool)
	if len(strings.TrimSpace(config.MutexName)) < 1 {
		return nil, errors.New("invalid mutext name, should be non-blank")
	}
	if config.Pool == nil {
		return nil, errors.New("a Redis pool is required")
	}

	mutexName := config.MutexName

	instance := redisSyncAdapter{
		pool:      config.Pool,
		redsync:   rs,
		mutexMap:  make(map[string]*redsync.Mutex),
		mutexName: mutexName,
	}

	return &instance, nil
}

func (instance *redisSyncAdapter) tenantMutex(tenantKey string) *redsync.Mutex {
	if existing, exists := instance.mutexMap[tenantKey]; exists {
		return existing
	}

	mutex := instance.redsync.NewMutex(
		instance.effectiveKey(tenantKey, "lock"),
		redsync.WithExpiry(5*time.Second),
	)

	instance.mutexMap[tenantKey] = mutex
	return mutex
}

func (instance *redisSyncAdapter) effectiveKey(tenantKey string, postfix string) string {
	return fmt.Sprintf("%s.%s.%s", instance.mutexName, url.QueryEscape(tenantKey), postfix)
}

func (instance *redisSyncAdapter) Lock(ctx context.Context, tenantKey string) error {
	instance.localMutext.Lock()

	err := instance.tenantMutex(tenantKey).Lock()
	if err != nil {
		instance.localMutext.Unlock()
	}
	return err
}

func (instance *redisSyncAdapter) Fetch(ctx context.Context, tenantKey string) (string, error) {
	c := context.Background()

	conn, err := instance.pool.Get(c)
	if err != nil {
		return "", err
	}

	val, err := conn.Get(instance.mutexName + ".data")

	if err == nil && val == "" {
		return "", nil
	} else if err != nil {
		return "", err
	} else {
		res := string(val)
		return res, nil
	}
}

func (instance *redisSyncAdapter) Write(ctx context.Context, tenantKey string, status string) error {
	c := context.Background()

	conn, err := instance.pool.Get(c)
	if err != nil {
		return err
	}

	ok, err := conn.Set(instance.mutexName+".data", status)
	if err != nil {
		return err
	}

	if !ok {
		return errors.New("write to server failed")
	}

	return err
}

func (instance *redisSyncAdapter) Unlock(ctx context.Context, tenantKey string) error {
	defer instance.localMutext.Unlock()

	_, err := instance.tenantMutex(tenantKey).Unlock()

	return err
}
