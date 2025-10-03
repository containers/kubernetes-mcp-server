package test

import (
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func KubeConfigFake() *clientcmdapi.Config {
	fakeConfig := clientcmdapi.NewConfig()
	fakeConfig.Clusters["fake"] = clientcmdapi.NewCluster()
	fakeConfig.Clusters["fake"].Server = "https://127.0.0.1:6443"
	fakeConfig.AuthInfos["fake"] = clientcmdapi.NewAuthInfo()
	fakeConfig.Contexts["fake-context"] = clientcmdapi.NewContext()
	fakeConfig.Contexts["fake-context"].Cluster = "fake"
	fakeConfig.Contexts["fake-context"].AuthInfo = "fake"
	fakeConfig.CurrentContext = "fake-context"
	return fakeConfig
}
