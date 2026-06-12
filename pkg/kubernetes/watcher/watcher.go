package watcher

import "context"

type Watcher interface {
	Watch(ctx context.Context, onChange func() error)
	Close()
}
