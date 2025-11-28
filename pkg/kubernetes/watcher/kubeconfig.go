package watcher

import (
	"github.com/fsnotify/fsnotify"
	"k8s.io/client-go/tools/clientcmd"
)

type Kubeconfig struct {
	clientcmd.ClientConfig
	close func() error
}

var _ Watcher = (*Kubeconfig)(nil)

func NewKubeconfig(clientConfig clientcmd.ClientConfig) *Kubeconfig {
	return &Kubeconfig{
		ClientConfig: clientConfig,
	}
}

func (w *Kubeconfig) Watch(onChange func() error) {
	kubeConfigFiles := w.ConfigAccess().GetLoadingPrecedence()
	if len(kubeConfigFiles) == 0 {
		return
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	for _, file := range kubeConfigFiles {
		_ = watcher.Add(file)
	}
	go func() {
		for {
			select {
			case _, ok := <-watcher.Events:
				if !ok {
					return
				}
				_ = onChange()
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	if w.close != nil {
		_ = w.close()
	}
	w.close = watcher.Close
}

func (w *Kubeconfig) Close() error {
	if w.close != nil {
		return w.close()
	}
	return nil
}
