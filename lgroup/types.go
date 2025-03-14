package lgroup

import (
	"context"
)

type LGroup[Init any] interface {
	Join(context.Context, Init) bool
	Register(func(context.Context, Init) error)
	Done() <-chan struct{}
}
