package watcher

type Watcher interface {
	Watch(onChange func() error)
	Close()
}
