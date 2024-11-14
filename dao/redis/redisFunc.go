package redis

import (
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

func DbErr(err error) error {
	if errors.Is(err, redis.Nil) {
		return nil
	}
	return err
}
